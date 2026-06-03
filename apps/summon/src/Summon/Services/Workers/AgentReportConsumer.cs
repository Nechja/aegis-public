using System.Text;
using System.Text.Json;
using RabbitMQ.Client;
using RabbitMQ.Client.Events;
using Summon.Models;
using Summon.Services.Data;

namespace Summon.Services.Workers;

public sealed class AgentReportConsumer(
    SummonDb db,
    VmStateNotifier notifier,
    FleetSnapshot fleet,
    FleetOperationService ops,
    IConfiguration config,
    ILogger<AgentReportConsumer> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var host = config["RABBITMQ_HOST"] ?? "rabbitmq.rabbitmq.svc.cluster.local";
        var port = int.Parse(config["RABBITMQ_PORT"] ?? "5672");
        var user = config["RABBITMQ_USER"] ?? "aegis";
        var pass = config["RABBITMQ_PASS"] ?? "guest";

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await ConsumeReports(host, port, user, pass, stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "Agent report consumer disconnected, reconnecting in 5s");
                await Task.Delay(TimeSpan.FromSeconds(5), stoppingToken);
            }
        }
    }

    private async Task ConsumeReports(string host, int port, string user, string pass, CancellationToken ct)
    {
        var factory = new ConnectionFactory
        {
            HostName = host,
            Port = port,
            UserName = user,
            Password = pass
        };

        await using var connection = await factory.CreateConnectionAsync(ct);
        await using var channel = await connection.CreateChannelAsync(cancellationToken: ct);

        logger.LogInformation("AgentReportConsumer connected to RabbitMQ");

        var knownQueues = new HashSet<string>();

        while (!ct.IsCancellationRequested)
        {
            var vms = db.Vms.FindAll()
                .Where(v => v.State is not VmState.Deleting)
                .ToList();

            foreach (var vm in vms)
            {
                var queue = $"summon.report.{vm.KubeVmName}";
                if (!knownQueues.Add(queue)) continue;

                await channel.QueueDeclareAsync(queue, durable: true, exclusive: false, autoDelete: false, cancellationToken: ct);

                var consumer = new AsyncEventingBasicConsumer(channel);
                consumer.ReceivedAsync += async (_, ea) =>
                {
                    try
                    {
                        var body = Encoding.UTF8.GetString(ea.Body.Span);
                        var report = JsonSerializer.Deserialize<AgentReport>(body, JsonOpts);

                        if (report is not null)
                            await HandleReport(report);

                        await channel.BasicAckAsync(ea.DeliveryTag, false, ct);
                    }
                    catch (Exception ex)
                    {
                        logger.LogWarning(ex, "Error processing agent report");
                        await channel.BasicNackAsync(ea.DeliveryTag, false, false, ct);
                    }
                };

                await channel.BasicConsumeAsync(queue, false, consumer, ct);
                logger.LogInformation("Consuming reports from {Queue}", queue);
            }

            await Task.Delay(TimeSpan.FromSeconds(10), ct);
        }
    }

    private async Task HandleReport(AgentReport report)
    {
        fleet.TouchAgent(report.VmName);

        logger.LogInformation("Agent report from {VM}: {Type} {Status} {PackageId} {Label}",
            report.VmName, report.Type, report.Status, report.PackageId, report.Label);

        var vm = db.Vms.FindOne(v => v.KubeVmName == report.VmName);
        if (vm is null) return;

        if (report.Type == "inventory.done")
        {
            vm.PackageManagerSnapshot = report.Output;
            vm.PackageManagerSnapshotAt = DateTime.UtcNow;
            if (report.Pending is { } pend)
            {
                vm.PendingAptUpdates = pend.AptUpgradable;
                vm.PendingSecurityUpdates = pend.AptSecurity;
                vm.PendingSnapRefreshes = pend.SnapRefreshable;
                if (pend.IndexRefreshed)
                    vm.LastIndexRefreshedAt = DateTime.UtcNow;
            }
            db.Vms.Update(vm);
        }

        if (report.Type == "agent.ready" && !string.IsNullOrWhiteSpace(report.AgentVersion))
        {
            vm.AgentVersion = report.AgentVersion;
            db.Vms.Update(vm);
        }

        if (report.Type == "facts.done" && report.Facts is { } f)
        {
            vm.Telemetry = new TelemetrySnapshot
            {
                Kernel = f.Kernel,
                OsPretty = f.OsPretty,
                UptimeSec = f.UptimeSec,
                CpuPercent = f.CpuPercent,
                CpuCount = f.CpuCount,
                LoadAvg1 = f.LoadAvg1,
                MemTotalKb = f.MemTotalKB,
                MemUsedKb = f.MemUsedKB,
                MemPercent = f.MemPercent,
                DiskTotalKb = f.DiskTotalKB,
                DiskUsedKb = f.DiskUsedKB,
                DiskPercent = f.DiskPercent,
                PrimaryIp = f.PrimaryIp,
                AgentVersion = f.AgentVersion ?? report.AgentVersion,
                CapturedAt = DateTime.UtcNow
            };
            if (!string.IsNullOrWhiteSpace(f.AgentVersion)) vm.AgentVersion = f.AgentVersion;
            db.Vms.Update(vm);
        }

        if (report.Type == "apps.list.done")
        {
            vm.InstalledApps = report.Apps ?? [];
            vm.InstalledAppsCapturedAt = DateTime.UtcNow;
            db.Vms.Update(vm);
        }

        if (report.Type == "uninstall.done" && !string.IsNullOrWhiteSpace(report.PackageId)
            && vm.SoftwareInstalls is { Count: > 0 } ledger)
        {
            // Trim the matching ledger row if the uninstall succeeded so the install ledger
            // reflects reality, not summon-history.
            if (string.Equals(report.Status, "removed", StringComparison.Ordinal))
            {
                ledger.RemoveAll(e => string.Equals(e.PackageKey, report.PackageId, StringComparison.Ordinal));
                db.Vms.Update(vm);
            }
        }

        // Fleet-op correlation: match the report to its operation target, update phase.
        var attach = ops.TryAttach(report);
        if (attach is { } a)
            ops.ApplyReportToTarget(a.Op, a.Target, report);

        var display = !string.IsNullOrWhiteSpace(report.Label)
            ? report.Label
            : (!string.IsNullOrWhiteSpace(report.PackageId) ? report.PackageId : report.Package ?? report.Type);

        ApplyInstallLedger(vm, report);

        var progress = report.Type switch
        {
            "agent.ready" => "Agent connected",
            "install.progress" => $"Installing: {display}",
            "install.done" when report.Status == "installed" => $"Installed: {display}",
            "install.done" when report.Status == "failed" => $"Failed: {display}",
            "exec.result" => $"Command finished (exit {report.ExitCode})",
            "inventory.done" when report.Status == "ok" => "APT / Snap snapshot updated",
            "inventory.done" => "APT / Snap snapshot failed",
            "facts.done" => "Facts updated",
            "apps.list.done" => $"Apps refreshed ({report.Apps?.Count ?? 0} packages)",
            "upgrade.progress" => "Upgrading…",
            "upgrade.done" when report.Status == "ok" => "Upgrade complete",
            "upgrade.done" => $"Upgrade failed (exit {report.ExitCode})",
            "uninstall.done" when report.Status == "removed" => $"Removed: {display}",
            "uninstall.done" => $"Remove failed: {display}",
            _ => null
        };

        if (progress is not null)
            await notifier.Publish(new VmStateEvent(vm.Id, vm.State, progress));

        if (report.Type is not "heartbeat" and not "install.progress")
        {
            var auditDetail = report.Type == "inventory.done"
                ? $"exit {report.ExitCode}, {report.Output?.Length ?? 0} chars"
                : $"{display}: {report.Status}";
            db.Audit.Insert(new AuditEntry
            {
                Action = $"agent.{report.Type}",
                ResourceId = vm.Id.ToString(),
                Detail = auditDetail
            });
        }
    }

    private void ApplyInstallLedger(VmRecord vm, AgentReport report)
    {
        var list = vm.SoftwareInstalls;
        if (list is null || list.Count == 0)
            return;

        var row = FindInstallRow(list, report);
        if (row is null)
            return;

        var now = DateTime.UtcNow;
        switch (report.Type)
        {
            case "install.progress":
                row.Phase = PackageInstallPhase.Installing;
                row.LastOutput = null;
                row.ExitCode = null;
                row.UpdatedAt = now;
                db.Vms.Update(vm);
                break;
            case "install.done":
                row.Phase = report.Status == "installed" ? PackageInstallPhase.Installed : PackageInstallPhase.Failed;
                row.LastOutput = report.Output;
                row.ExitCode = report.ExitCode;
                row.UpdatedAt = now;
                db.Vms.Update(vm);
                break;
        }
    }

    private static PackageInstallEntry? FindInstallRow(List<PackageInstallEntry> list, AgentReport report)
    {
        if (!string.IsNullOrWhiteSpace(report.PackageId))
        {
            var byId = list.FirstOrDefault(e =>
                string.Equals(e.PackageKey, report.PackageId, StringComparison.OrdinalIgnoreCase));
            if (byId is not null)
                return byId;
        }

        if (!string.IsNullOrWhiteSpace(report.Package))
        {
            var needle = NormalizeSnippet(report.Package);
            return list.FirstOrDefault(e => NormalizeSnippet(e.ScriptSnippet) == needle);
        }

        return null;
    }

    private static string NormalizeSnippet(string? s) => (s ?? string.Empty).Trim();

    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNameCaseInsensitive = true
    };
}
