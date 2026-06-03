using System.Text.Json;
using Cronos;
using k8s;
using k8s.Models;
using Microsoft.Extensions.Options;
using Summon.Models;
using Summon.Services.Data;

namespace Summon.Services.Workers;

/// <summary>
/// Three-way reconciler between the KubeVirt cluster state and the LiteDB <see cref="VmRecord"/> collection.
/// Ticks on a Cronos schedule and handles:
/// <list type="bullet">
///   <item><description><b>Adopt</b> — VM in cluster but not in DB: create a <see cref="VmRecord"/>, reading back
///   <c>summon.aegis/*</c> annotations if present.</description></item>
///   <item><description><b>Prune</b> — VM in DB but not in cluster, past grace period: delete the record.</description></item>
///   <item><description><b>Sync</b> — both present: update VMI-derived state and Tailscale IP.</description></item>
/// </list>
/// </summary>
public sealed class FleetReconciler(
    KubeClientFactory kube,
    SummonDb db,
    VmStateNotifier notifier,
    IOptions<ReconcilerOptions> options,
    ILogger<FleetReconciler> logger) : BackgroundService
{
    private const string VmGroup = "kubevirt.io";
    private const string VmVersion = "v1";
    private const string VmPlural = "virtualmachines";
    private const string VmiPlural = "virtualmachineinstances";

    private readonly ReconcilerOptions _opts = options.Value;

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        CronExpression cron;
        try
        {
            cron = CronExpression.Parse(_opts.Schedule, CronFormat.IncludeSeconds);
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Invalid reconciler schedule '{Schedule}'; FleetReconciler disabled", _opts.Schedule);
            return;
        }

        logger.LogInformation(
            "FleetReconciler started. Schedule={Schedule} Namespace={Namespace} Grace={Grace}",
            _opts.Schedule, _opts.Namespace, _opts.GracePeriod);

        while (!stoppingToken.IsCancellationRequested)
        {
            var now = DateTime.UtcNow;
            var next = cron.GetNextOccurrence(now);
            if (next is null)
            {
                logger.LogWarning("Cron expression has no future occurrence; exiting FleetReconciler loop");
                return;
            }

            var delay = next.Value - now;
            if (delay < TimeSpan.Zero) delay = TimeSpan.Zero;

            try
            {
                await Task.Delay(delay, stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }

            try
            {
                await ReconcileOnce(stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                // Swallow — one bad tick must not kill the loop.
                logger.LogError(ex, "FleetReconciler tick failed");
            }
        }
    }

    internal async Task ReconcileOnce(CancellationToken ct)
    {
        var ns = _opts.Namespace;

        // 1. Cluster state — VMs, VMIs (for phase), Services (for Tailscale IP).
        var vmCrs = await ListCustomObject(VmGroup, VmVersion, ns, VmPlural, ct);
        var vmiCrs = await ListCustomObject(VmGroup, VmVersion, ns, VmiPlural, ct);
        V1ServiceList services;
        try
        {
            services = await kube.Client.CoreV1.ListNamespacedServiceAsync(ns, cancellationToken: ct);
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Failed to list services in {Namespace}; continuing without Tailscale hydration", ns);
            services = new V1ServiceList { Items = [] };
        }

        var vmiByName = vmiCrs;
        var clusterVmByName = vmCrs;
        var serviceByName = services.Items.ToDictionary(s => s.Metadata?.Name ?? string.Empty, s => s, StringComparer.Ordinal);

        // 2. DB state.
        var dbVms = db.Vms.FindAll().ToList();
        var dbByKubeName = dbVms
            .Where(v => !string.IsNullOrEmpty(v.KubeVmName))
            .GroupBy(v => v.KubeVmName, StringComparer.Ordinal)
            .ToDictionary(g => g.Key, g => g.First(), StringComparer.Ordinal);

        // 3. ADOPT — in cluster, not in DB.
        foreach (var (name, crDoc) in clusterVmByName)
        {
            if (dbByKubeName.ContainsKey(name)) continue;
            try
            {
                var record = BuildAdoptedRecord(name, crDoc, vmiByName, serviceByName, ns);
                db.Vms.Insert(record);
                logger.LogInformation("Adopted external VM {Name} (state={State})", record.Name, record.State);
                await notifier.Publish(new VmStateEvent(record.Id, record.State));
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "Failed to adopt {Name}", name);
            }
        }

        // 4. PRUNE — in DB, not in cluster (past grace period).
        var now = DateTime.UtcNow;
        foreach (var record in dbVms)
        {
            if (string.IsNullOrEmpty(record.KubeVmName)) continue;
            if (clusterVmByName.ContainsKey(record.KubeVmName)) continue;
            if (now - record.CreatedAt <= _opts.GracePeriod) continue;
            if (record.State == VmState.Deleting) continue; // already tombstoned by something else

            try
            {
                db.Vms.Delete(record.Id);
                logger.LogInformation("Pruned ghost VM record {Name} ({KubeVmName})", record.Name, record.KubeVmName);
                await notifier.Publish(new VmStateEvent(record.Id, VmState.Deleting));
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "Failed to prune {Name}", record.Name);
            }
        }

        // 5. SYNC — present on both sides.
        foreach (var record in dbVms)
        {
            if (string.IsNullOrEmpty(record.KubeVmName)) continue;
            if (!clusterVmByName.ContainsKey(record.KubeVmName)) continue;

            try
            {
                var mutated = false;

                // State from VMI phase (if a VMI exists).
                if (vmiByName.TryGetValue(record.KubeVmName, out var vmiDoc))
                {
                    var phase = ReadVmiPhase(vmiDoc);
                    var mapped = MapVmiPhase(phase);
                    if (mapped is not null && record.State != mapped)
                    {
                        record.State = mapped;
                        mutated = true;
                    }
                }

                // Tailscale IP from matching service.
                var svcName = $"vm-{record.KubeVmName}";
                if (serviceByName.TryGetValue(svcName, out var svc))
                {
                    var ip = ReadLoadBalancerIp(svc);
                    if (!string.IsNullOrWhiteSpace(ip) && record.TailscaleIp != ip)
                    {
                        record.TailscaleIp = ip!;
                        mutated = true;
                    }
                }

                if (mutated)
                {
                    db.Vms.Update(record);
                    await notifier.Publish(new VmStateEvent(record.Id, record.State));
                }
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "Failed to sync {Name}", record.Name);
            }
        }
    }

    private async Task<Dictionary<string, JsonDocument>> ListCustomObject(
        string group, string version, string ns, string plural, CancellationToken ct)
    {
        var map = new Dictionary<string, JsonDocument>(StringComparer.Ordinal);
        try
        {
            var result = await kube.Client.CustomObjects.ListNamespacedCustomObjectAsync(
                group, version, ns, plural, cancellationToken: ct);

            var doc = JsonSerializer.SerializeToDocument(result);
            if (doc.RootElement.TryGetProperty("items", out var items) && items.ValueKind == JsonValueKind.Array)
            {
                foreach (var item in items.EnumerateArray())
                {
                    var name = item.TryGetProperty("metadata", out var meta)
                        && meta.TryGetProperty("name", out var n)
                            ? n.GetString()
                            : null;
                    if (string.IsNullOrEmpty(name)) continue;

                    // Clone each element to its own JsonDocument so it outlives the parent `doc`.
                    map[name] = JsonDocument.Parse(item.GetRawText());
                }
            }
            doc.Dispose();
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Failed to list {Plural} in {Namespace}", plural, ns);
        }
        return map;
    }

    private VmRecord BuildAdoptedRecord(
        string kubeVmName,
        JsonDocument crDoc,
        Dictionary<string, JsonDocument> vmiByName,
        Dictionary<string, V1Service> serviceByName,
        string ns)
    {
        var root = crDoc.RootElement;

        var annotations = TryGetObject(root, "metadata", "annotations");
        var template = TryGetString(annotations, "summon.aegis/template") ?? "Unknown";
        var instanceType = TryGetString(annotations, "summon.aegis/instance-type") ?? "unknown";
        var diskGb = 0;
        if (int.TryParse(TryGetString(annotations, "summon.aegis/disk-gb"), out var parsedDisk) && parsedDisk > 0)
        {
            diskGb = parsedDisk;
        }
        var expiresAtRaw = TryGetString(annotations, "summon.aegis/expires-at");
        DateTime? expiresAt = null;
        if (!string.IsNullOrWhiteSpace(expiresAtRaw) && expiresAtRaw != "none"
            && DateTime.TryParse(expiresAtRaw, null, System.Globalization.DateTimeStyles.RoundtripKind, out var parsed))
        {
            expiresAt = parsed;
        }

        var packages = new List<string>();
        var packagesRaw = TryGetString(annotations, "summon.aegis/packages");
        if (!string.IsNullOrWhiteSpace(packagesRaw))
        {
            try
            {
                var parsedList = JsonSerializer.Deserialize<List<string>>(packagesRaw);
                if (parsedList is not null) packages = parsedList;
            }
            catch (Exception ex)
            {
                logger.LogDebug(ex, "Could not parse summon.aegis/packages for {Name}", kubeVmName);
            }
        }

        var state = VmState.Creating;
        if (vmiByName.TryGetValue(kubeVmName, out var vmiDoc))
        {
            var phase = ReadVmiPhase(vmiDoc);
            state = MapVmiPhase(phase) ?? VmState.Creating;
        }

        string tailscaleIp = string.Empty;
        if (serviceByName.TryGetValue($"vm-{kubeVmName}", out var svc))
        {
            tailscaleIp = ReadLoadBalancerIp(svc) ?? string.Empty;
        }

        return new VmRecord
        {
            Name = kubeVmName,
            TemplateName = template,
            InstanceType = instanceType,
            DiskGb = diskGb,
            KubeNamespace = ns,
            KubeVmName = kubeVmName,
            TailscaleHostname = kubeVmName,
            TailscaleIp = tailscaleIp,
            Packages = packages,
            CloudInit = string.Empty,
            ExpiresAt = expiresAt,
            State = state,
            ExternalOrigin = true
        };
    }

    private static string? ReadVmiPhase(JsonDocument vmiDoc)
    {
        if (!vmiDoc.RootElement.TryGetProperty("status", out var status)) return null;
        if (!status.TryGetProperty("phase", out var phase)) return null;
        return phase.ValueKind == JsonValueKind.String ? phase.GetString() : null;
    }

    private static string? MapVmiPhase(string? phase) => phase switch
    {
        "Pending" or "Scheduling" or "Scheduled" => VmState.Starting,
        "Running" => VmState.Running,
        "Succeeded" => VmState.Stopped,
        "Failed" => VmState.Failed,
        _ => null
    };

    private static string? ReadLoadBalancerIp(V1Service svc)
    {
        var ingress = svc.Status?.LoadBalancer?.Ingress;
        if (ingress is null || ingress.Count == 0) return null;
        var first = ingress[0];
        return !string.IsNullOrWhiteSpace(first.Ip) ? first.Ip
             : !string.IsNullOrWhiteSpace(first.Hostname) ? first.Hostname
             : null;
    }

    private static JsonElement TryGetObject(JsonElement parent, params string[] path)
    {
        var current = parent;
        foreach (var seg in path)
        {
            if (current.ValueKind != JsonValueKind.Object) return default;
            if (!current.TryGetProperty(seg, out var next)) return default;
            current = next;
        }
        return current;
    }

    private static string? TryGetString(JsonElement obj, string key)
    {
        if (obj.ValueKind != JsonValueKind.Object) return null;
        if (!obj.TryGetProperty(key, out var v)) return null;
        return v.ValueKind == JsonValueKind.String ? v.GetString() : null;
    }
}
