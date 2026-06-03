using System.Collections.Concurrent;
using Summon.Models;
using Summon.Services.Data;

namespace Summon.Services;

public sealed record FleetStats(
    int TotalVms,
    int Running,
    int Creating,
    int Failed,
    int Stopped,
    int AgentsLive,
    DateTime? LastAgentEvent);

public sealed class FleetSnapshot(SummonDb db)
{
    private static readonly TimeSpan LivenessWindow = TimeSpan.FromSeconds(90);
    private readonly ConcurrentDictionary<string, DateTime> _lastSeen = new(StringComparer.Ordinal);

    public void TouchAgent(string kubeVmName)
    {
        if (string.IsNullOrWhiteSpace(kubeVmName)) return;
        _lastSeen[kubeVmName] = DateTime.UtcNow;
    }

    public FleetStats Snapshot()
    {
        var vms = db.Vms.FindAll().ToList();
        var cutoff = DateTime.UtcNow - LivenessWindow;
        var live = vms.Count(v => _lastSeen.TryGetValue(v.KubeVmName, out var t) && t > cutoff);
        DateTime? last = _lastSeen.IsEmpty ? null : _lastSeen.Values.Max();

        return new FleetStats(
            TotalVms: vms.Count,
            Running: vms.Count(v => v.State == VmState.Running),
            Creating: vms.Count(v => v.State is VmState.Creating or VmState.Importing or VmState.Starting),
            Failed: vms.Count(v => v.State == VmState.Failed),
            Stopped: vms.Count(v => v.State == VmState.Stopped),
            AgentsLive: live,
            LastAgentEvent: last);
    }
}
