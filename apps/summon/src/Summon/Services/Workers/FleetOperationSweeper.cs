namespace Summon.Services.Workers;

/// <summary>Periodically calls <see cref="FleetOperationService.Sweep"/> to force-fail stale targets and abandon ghost ops.</summary>
public sealed class FleetOperationSweeper(
    FleetOperationService ops,
    ILogger<FleetOperationSweeper> logger) : BackgroundService
{
    private static readonly TimeSpan Interval = TimeSpan.FromMinutes(5);

    protected override async Task ExecuteAsync(CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try { ops.Sweep(); }
            catch (Exception ex) { logger.LogWarning(ex, "fleet-op sweep failed"); }

            try { await Task.Delay(Interval, ct); }
            catch (OperationCanceledException) { return; }
        }
    }
}
