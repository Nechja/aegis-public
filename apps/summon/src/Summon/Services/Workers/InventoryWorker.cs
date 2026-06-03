using Summon.Models;
using Summon.Services.Data;

namespace Summon.Services.Workers;

/// <summary>
/// Periodically asks each Running VM's agent for its pending-update counts.
/// Cached probes run on a fast cadence; a slow cadence triggers <c>inventory.refresh</c>
/// (apt-get update) so counts don't drift indefinitely.
/// </summary>
public sealed class InventoryWorker(
    SummonDb db,
    AgentMessaging agent,
    IConfiguration config,
    ILogger<InventoryWorker> logger) : BackgroundService
{
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var probeMinutes = config.GetValue("INVENTORY_PROBE_MINUTES", 30);
        var refreshHours = config.GetValue("INVENTORY_REFRESH_HOURS", 12);
        var stagger = TimeSpan.FromSeconds(config.GetValue("INVENTORY_STAGGER_SECONDS", 3));

        var probeInterval = TimeSpan.FromMinutes(probeMinutes);
        var refreshInterval = TimeSpan.FromHours(refreshHours);

        logger.LogInformation("InventoryWorker started: probe every {ProbeMin}m, refresh every {RefreshHr}h",
            probeMinutes, refreshHours);

        // Brief warm-up so AgentMessaging has its connection up.
        await SafeDelay(TimeSpan.FromSeconds(15), stoppingToken);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await ProbeAll(refreshInterval, stagger, stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "InventoryWorker tick failed");
            }

            await SafeDelay(probeInterval, stoppingToken);
        }
    }

    private async Task ProbeAll(TimeSpan refreshInterval, TimeSpan stagger, CancellationToken ct)
    {
        var now = DateTime.UtcNow;
        var vms = db.Vms.FindAll()
            .Where(v => v.State == VmState.Running && !string.IsNullOrWhiteSpace(v.KubeVmName))
            .ToList();

        if (vms.Count == 0) return;

        foreach (var vm in vms)
        {
            if (ct.IsCancellationRequested) return;

            var needsRefresh = vm.LastIndexRefreshedAt is null
                || now - vm.LastIndexRefreshedAt.Value >= refreshInterval;

            try
            {
                if (needsRefresh)
                    await agent.SendInventoryRefresh(vm.KubeVmName);
                else
                    await agent.SendInventoryProbe(vm.KubeVmName);
            }
            catch (Exception ex)
            {
                logger.LogWarning(ex, "Inventory probe to {Vm} failed", vm.KubeVmName);
            }

            await SafeDelay(stagger, ct);
        }
    }

    private static async Task SafeDelay(TimeSpan delay, CancellationToken ct)
    {
        try { await Task.Delay(delay, ct); }
        catch (OperationCanceledException) { }
    }
}
