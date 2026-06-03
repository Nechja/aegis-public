using System.Text.Json;
using k8s;
using Summon.Models;
using Summon.Services.Data;

namespace Summon.Services.Workers;

public sealed class KubeWatcher(
    KubeClientFactory kube,
    SummonDb db,
    VmStateNotifier notifier,
    ILogger<KubeWatcher> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        logger.LogInformation("KubeWatcher started, watching namespace {Ns}", kube.VmNamespace);

        await Task.WhenAll(
            WatchLoop("VMI", WatchVmInstances, stoppingToken),
            WatchLoop("DataVolume", WatchDataVolumes, stoppingToken));
    }

    private async Task WatchLoop(string name, Func<CancellationToken, Task> watch, CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try
            {
                await watch(ct);
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "{Name} watch disconnected, reconnecting in 5s", name);
                await Task.Delay(TimeSpan.FromSeconds(5), ct);
            }
        }
    }

    private async Task WatchVmInstances(CancellationToken ct)
    {
        var response = kube.Client.CustomObjects.ListNamespacedCustomObjectWithHttpMessagesAsync(
            "kubevirt.io", "v1", kube.VmNamespace, "virtualmachineinstances",
            watch: true, cancellationToken: ct);

#pragma warning disable CS0618
        await foreach (var (type, item) in response.WatchAsync<object, object>(
            onError: ex => logger.LogWarning(ex, "VMI watch error"), cancellationToken: ct))
#pragma warning restore CS0618
        {
            try
            {
                using var doc = JsonSerializer.SerializeToDocument(item);
                var root = doc.RootElement;

                var name = root.GetProperty("metadata").GetProperty("name").GetString();
                var phase = root.TryGetProperty("status", out var status)
                    ? status.GetProperty("phase").GetString()
                    : null;

                if (name is null) continue;

                logger.LogInformation("VMI {Name} phase: {Phase} (event: {Type})", name, phase, type);

                var newState = MapVmiPhase(type, phase);
                if (newState is null) continue;

                await UpdateVmState(name, newState, ct);
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "Error processing VMI event");
            }
        }
    }

    private async Task WatchDataVolumes(CancellationToken ct)
    {
        var response = kube.Client.CustomObjects.ListNamespacedCustomObjectWithHttpMessagesAsync(
            "cdi.kubevirt.io", "v1beta1", kube.VmNamespace, "datavolumes",
            watch: true, cancellationToken: ct);

#pragma warning disable CS0618
        await foreach (var (type, item) in response.WatchAsync<object, object>(
            onError: ex => logger.LogWarning(ex, "DV watch error"), cancellationToken: ct))
#pragma warning restore CS0618
        {
            try
            {
                using var doc = JsonSerializer.SerializeToDocument(item);
                var root = doc.RootElement;

                var dvName = root.GetProperty("metadata").GetProperty("name").GetString();
                if (dvName is null || !dvName.EndsWith("-disk")) continue;

                var vmName = dvName[..^5]; // strip "-disk"

                string? phase = null;
                string? progress = null;
                if (root.TryGetProperty("status", out var status) && status.ValueKind == JsonValueKind.Object)
                {
                    if (status.TryGetProperty("phase", out var p)) phase = p.GetString();
                    if (status.TryGetProperty("progress", out var prog)) progress = prog.GetString();
                }

                logger.LogInformation("DV {Name} phase: {Phase} progress: {Progress}", dvName, phase, progress);

                var newState = MapDvPhase(phase);
                if (newState is null) continue;

                await UpdateVmState(vmName, newState, ct, progress);
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "Error processing DataVolume event");
            }
        }
    }

    private async Task UpdateVmState(string kubeVmName, string newState, CancellationToken ct, string? progress = null)
    {
        var vm = db.Vms.FindOne(v => v.KubeVmName == kubeVmName);
        if (vm is null) return;

        if (vm.State == newState && progress is null) return;

        vm.State = newState;
        db.Vms.Update(vm);

        logger.LogInformation("Updated {Name} state to {State}", vm.Name, newState);

        await notifier.Publish(new VmStateEvent(vm.Id, newState, progress));
    }

    private static string? MapVmiPhase(WatchEventType type, string? phase) => type switch
    {
        WatchEventType.Deleted => VmState.Stopped,
        WatchEventType.Added or WatchEventType.Modified => phase switch
        {
            "Pending" or "Scheduling" or "Scheduled" => VmState.Starting,
            "Running" => VmState.Running,
            "Succeeded" => VmState.Stopped,
            "Failed" => VmState.Failed,
            _ => null
        },
        _ => null
    };

    private static string? MapDvPhase(string? phase) => phase switch
    {
        "ImportScheduled" => VmState.Importing,
        "ImportInProgress" => VmState.Importing,
        "Succeeded" => VmState.Starting,
        "Failed" => VmState.Failed,
        _ => null
    };
}
