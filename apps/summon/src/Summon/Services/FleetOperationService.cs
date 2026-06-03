using Summon.Models;
using Summon.Services.Data;

namespace Summon.Services;

/// <summary>
/// Tracks multi-VM operations (upgrade / install / uninstall / apps.list). Creates a
/// <see cref="FleetOperation"/> in LiteDB, fans out the command per target VM, correlates
/// incoming agent reports back to the operation, and updates per-target phase.
/// </summary>
public sealed class FleetOperationService(
    SummonDb db,
    AgentMessaging agent,
    VmStateNotifier notifier,
    ILogger<FleetOperationService> logger)
{
    private static readonly TimeSpan StaleTargetTimeout = TimeSpan.FromMinutes(90);
    private static readonly TimeSpan GhostOpTimeout = TimeSpan.FromMinutes(90);

    public async Task<Guid> StartUpgrade(IReadOnlyList<Guid> vmIds)
    {
        var (op, targets) = Begin(FleetOperationKind.Upgrade, vmIds, "apt upgrade", requiresAptSlot: true);
        foreach (var t in targets)
        {
            try
            {
                await agent.SendUpgrade(t.VmName, op.Id.ToString());
                MarkTarget(op, t, FleetOperationTargetPhase.Running, "queued");
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "SendUpgrade {Vm} failed", t.VmName);
                MarkTarget(op, t, FleetOperationTargetPhase.Failed, $"send failed: {ex.Message}");
            }
        }
        CloseIfAllDone(op);
        return op.Id;
    }

    public async Task<Guid> StartInstall(IReadOnlyList<Guid> vmIds, IReadOnlyList<InstallCommandItem> items)
    {
        var summary = $"install {string.Join(", ", items.Select(i => i.Label))}";
        var (op, targets) = Begin(FleetOperationKind.Install, vmIds, summary, requiresAptSlot: true);
        foreach (var t in targets)
        {
            try
            {
                await agent.SendInstallItems(t.VmName, items, op.Id.ToString());
                MarkTarget(op, t, FleetOperationTargetPhase.Running, "queued");
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "SendInstallItems {Vm} failed", t.VmName);
                MarkTarget(op, t, FleetOperationTargetPhase.Failed, $"send failed: {ex.Message}");
            }
        }
        CloseIfAllDone(op);
        return op.Id;
    }

    /// <summary>
    /// Per-VM item lists — uninstalling a specific package may resolve to different install scripts
    /// on different VMs (apt vs snap), so the caller supplies a script per target.
    /// </summary>
    public async Task<Guid> StartUninstall(IReadOnlyList<Guid> vmIds, IReadOnlyDictionary<Guid, IReadOnlyList<InstallCommandItem>> itemsByVm)
    {
        var anyItems = itemsByVm.Values.FirstOrDefault();
        var summary = $"uninstall {(anyItems is null ? "(none)" : string.Join(", ", anyItems.Select(i => i.Label)))}";
        var (op, targets) = Begin(FleetOperationKind.Uninstall, vmIds, summary, requiresAptSlot: true);
        foreach (var t in targets)
        {
            if (!itemsByVm.TryGetValue(t.VmId, out var items) || items.Count == 0)
            {
                MarkTarget(op, t, FleetOperationTargetPhase.Done, "no matching items");
                continue;
            }
            try
            {
                await agent.SendUninstall(t.VmName, items, op.Id.ToString());
                MarkTarget(op, t, FleetOperationTargetPhase.Running, "queued");
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "SendUninstall {Vm} failed", t.VmName);
                MarkTarget(op, t, FleetOperationTargetPhase.Failed, $"send failed: {ex.Message}");
            }
        }
        CloseIfAllDone(op);
        return op.Id;
    }

    /// <summary>
    /// Fan a single bash script out to a set of VMs as a fleet exec op. Each VM runs the script
    /// independently; a per-target <c>exec.result</c> reports back exit code and (truncated) output.
    /// Doesn't take an apt slot — exec is an arbitrary script and may not touch apt.
    /// </summary>
    public async Task<Guid> StartExec(IReadOnlyList<Guid> vmIds, string script, string? label = null)
    {
        var summary = string.IsNullOrWhiteSpace(label) ? $"exec ({Truncate(script, 60)})" : $"exec: {label}";
        var (op, targets) = Begin(FleetOperationKind.Exec, vmIds, summary, requiresAptSlot: false);
        foreach (var t in targets)
        {
            try
            {
                await agent.SendExecCommand(t.VmName, script, op.Id.ToString());
                MarkTarget(op, t, FleetOperationTargetPhase.Running, "queued");
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "SendExecCommand {Vm} failed", t.VmName);
                MarkTarget(op, t, FleetOperationTargetPhase.Failed, $"send failed: {ex.Message}");
            }
        }
        CloseIfAllDone(op);
        return op.Id;
    }

    private static string Truncate(string s, int max) =>
        s.Length <= max ? s : s[..max] + "…";

    public async Task<Guid> StartRefreshApps(IReadOnlyList<Guid> vmIds)
    {
        var (op, targets) = Begin(FleetOperationKind.AppsList, vmIds, "refresh apps", requiresAptSlot: false);
        foreach (var t in targets)
        {
            try
            {
                await agent.SendAppsList(t.VmName, op.Id.ToString());
                MarkTarget(op, t, FleetOperationTargetPhase.Running, "queued");
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "SendAppsList {Vm} failed", t.VmName);
                MarkTarget(op, t, FleetOperationTargetPhase.Failed, $"send failed: {ex.Message}");
            }
        }
        CloseIfAllDone(op);
        return op.Id;
    }

    /// <summary>
    /// Find the operation + target this report belongs to. Tries <c>OperationId</c> first,
    /// falls back to the most recent in-flight op of a matching kind for the VM.
    /// Returns null for unsolicited reports (installs triggered outside a fleet op, etc.).
    /// </summary>
    public (FleetOperation Op, FleetOperationTarget Target)? TryAttach(AgentReport report)
    {
        if (!string.IsNullOrWhiteSpace(report.OperationId) && Guid.TryParse(report.OperationId, out var opId))
        {
            var op = db.Operations.FindById(opId);
            var target = op?.Targets.FirstOrDefault(t => string.Equals(t.VmName, report.VmName, StringComparison.Ordinal));
            if (op is not null && target is not null) return (op, target);
        }

        var kind = KindFromReportType(report.Type);
        if (kind is null) return null;

        var candidate = db.Operations
            .Find(o => o.CompletedAt == null && o.Kind == kind)
            .OrderByDescending(o => o.StartedAt)
            .FirstOrDefault(o => o.Targets.Any(t =>
                string.Equals(t.VmName, report.VmName, StringComparison.Ordinal) &&
                (t.Phase == FleetOperationTargetPhase.Pending || t.Phase == FleetOperationTargetPhase.Running)));
        if (candidate is null) return null;
        var fallbackTarget = candidate.Targets.First(t => t.VmName == report.VmName);
        return (candidate, fallbackTarget);
    }

    public void ApplyReportToTarget(FleetOperation op, FleetOperationTarget target, AgentReport report)
    {
        var (phase, message) = ResolvePhase(report);
        if (phase is null) return; // progress-style event without a terminal transition

        MarkTarget(op, target, phase, message, report.ExitCode, report.Output);
        CloseIfAllDone(op);
    }

    /// <summary>Returns true if the VM has an in-flight upgrade / install / uninstall op — UI should block new ones.</summary>
    public bool IsVmBusyWithAptOp(string vmName)
    {
        var open = db.Operations.Find(o => o.CompletedAt == null);
        return open.Any(o =>
            (o.Kind == FleetOperationKind.Upgrade
             || o.Kind == FleetOperationKind.Install
             || o.Kind == FleetOperationKind.Uninstall) &&
            o.Targets.Any(t => t.VmName == vmName &&
                (t.Phase == FleetOperationTargetPhase.Pending || t.Phase == FleetOperationTargetPhase.Running)));
    }

    public IReadOnlyList<FleetOperation> ActiveOperations() =>
        db.Operations.Find(o => o.CompletedAt == null)
            .OrderByDescending(o => o.StartedAt)
            .ToList();

    public IReadOnlyList<FleetOperation> RecentOperations(int take = 10) =>
        db.Operations.FindAll()
            .OrderByDescending(o => o.StartedAt)
            .Take(take)
            .ToList();

    /// <summary>
    /// Called periodically: force-fails targets whose phase is still pending/running past
    /// <see cref="StaleTargetTimeout"/> and closes ops that had nothing inserted. Also
    /// abandons ghost ops (inserted but no progress) older than <see cref="GhostOpTimeout"/>.
    /// </summary>
    public void Sweep()
    {
        var now = DateTime.UtcNow;
        var open = db.Operations.Find(o => o.CompletedAt == null).ToList();
        foreach (var op in open)
        {
            var anyChanged = false;
            foreach (var t in op.Targets)
            {
                if (t.Phase is FleetOperationTargetPhase.Pending or FleetOperationTargetPhase.Running
                    && now - t.UpdatedAt > StaleTargetTimeout)
                {
                    t.Phase = FleetOperationTargetPhase.Failed;
                    t.Message = "stale — no heartbeat";
                    t.UpdatedAt = now;
                    anyChanged = true;
                }
            }
            if (anyChanged) db.Operations.Update(op);

            if (now - op.StartedAt > GhostOpTimeout && op.Targets.All(t => t.Phase == FleetOperationTargetPhase.Pending))
            {
                foreach (var t in op.Targets) t.Phase = FleetOperationTargetPhase.Abandoned;
                op.CompletedAt = now;
                db.Operations.Update(op);
                continue;
            }

            CloseIfAllDone(op);
        }
    }

    // ---- helpers ----

    private (FleetOperation Op, List<FleetOperationTarget> Targets) Begin(
        string kind, IReadOnlyList<Guid> vmIds, string summary, bool requiresAptSlot)
    {
        var vms = vmIds.Select(id => db.Vms.FindById(id))
            .Where(v => v is not null)
            .Select(v => v!)
            .ToList();

        var targets = vms.Select(v =>
        {
            var busy = requiresAptSlot && IsVmBusyWithAptOp(v.KubeVmName);
            return new FleetOperationTarget
            {
                VmId = v.Id,
                VmName = v.KubeVmName,
                Phase = busy ? FleetOperationTargetPhase.Failed : FleetOperationTargetPhase.Pending,
                Message = busy ? "already has an apt-touching op in flight" : null,
                UpdatedAt = DateTime.UtcNow
            };
        }).ToList();

        var op = new FleetOperation
        {
            Kind = kind,
            Summary = summary,
            Targets = targets
        };
        db.Operations.Insert(op);
        logger.LogInformation("FleetOperation {Id} {Kind} created with {Count} targets", op.Id, kind, targets.Count);

        foreach (var t in targets)
        {
            db.Audit.Insert(new Summon.Models.AuditEntry
            {
                Action = $"fleet.{kind}.start",
                ResourceId = t.VmId.ToString(),
                Detail = $"{summary} (op {op.Id})"
            });
        }
        return (op, targets.Where(t => t.Phase == FleetOperationTargetPhase.Pending).ToList());
    }

    private void MarkTarget(FleetOperation op, FleetOperationTarget target, string phase, string? message, int? exitCode = null, string? output = null)
    {
        target.Phase = phase;
        target.Message = message;
        target.ExitCode = exitCode;
        if (!string.IsNullOrWhiteSpace(output)) target.Output = output;
        target.UpdatedAt = DateTime.UtcNow;
        db.Operations.Update(op);

        _ = notifier.Publish(new VmStateEvent(target.VmId, GetVmState(target.VmId), FormatProgress(op, target)));
    }

    private string GetVmState(Guid vmId) => db.Vms.FindById(vmId)?.State ?? VmState.Running;

    private static string FormatProgress(FleetOperation op, FleetOperationTarget target) =>
        $"{op.Kind}: {target.Phase}{(target.Message is null ? "" : $" · {target.Message}")}";

    private void CloseIfAllDone(FleetOperation op)
    {
        if (op.CompletedAt is not null) return;
        if (op.Targets.All(t => t.Phase is FleetOperationTargetPhase.Done
                                  or FleetOperationTargetPhase.Failed
                                  or FleetOperationTargetPhase.Abandoned))
        {
            op.CompletedAt = DateTime.UtcNow;
            db.Operations.Update(op);
            logger.LogInformation("FleetOperation {Id} closed", op.Id);
        }
    }

    private static string? KindFromReportType(string reportType) => reportType switch
    {
        "install.progress" or "install.done" => FleetOperationKind.Install,
        "uninstall.done" => FleetOperationKind.Uninstall,
        "upgrade.progress" or "upgrade.done" => FleetOperationKind.Upgrade,
        "apps.list.done" => FleetOperationKind.AppsList,
        "exec.result" => FleetOperationKind.Exec,
        _ => null
    };

    private static (string? Phase, string? Message) ResolvePhase(AgentReport r) => r.Type switch
    {
        "install.progress" or "upgrade.progress"
            => (FleetOperationTargetPhase.Running, null),
        "install.done"
            => (r.Status == "installed" ? FleetOperationTargetPhase.Done : FleetOperationTargetPhase.Failed,
                r.Status == "installed" ? null : $"install failed (exit {r.ExitCode})"),
        "uninstall.done"
            => (r.Status == "removed" ? FleetOperationTargetPhase.Done : FleetOperationTargetPhase.Failed,
                r.Status == "removed" ? null : $"uninstall failed (exit {r.ExitCode})"),
        "upgrade.done"
            => (r.Status == "ok" ? FleetOperationTargetPhase.Done : FleetOperationTargetPhase.Failed,
                r.Status == "ok" ? null : $"upgrade failed (exit {r.ExitCode})"),
        "apps.list.done"
            => (FleetOperationTargetPhase.Done, $"{r.Apps?.Count ?? 0} packages"),
        "exec.result"
            => (r.ExitCode == 0 ? FleetOperationTargetPhase.Done : FleetOperationTargetPhase.Failed,
                $"exit {r.ExitCode}"),
        _ => (null, null)
    };
}
