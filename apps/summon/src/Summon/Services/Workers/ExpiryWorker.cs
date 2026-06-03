using Summon.Models;
using Summon.Services.Data;

namespace Summon.Services.Workers;

public sealed class ExpiryWorker(
    SummonDb db,
    KubeVirtService kubeVirt,
    VmStateNotifier notifier,
    IConfiguration config,
    ILogger<ExpiryWorker> logger) : BackgroundService
{
    private static readonly TimeSpan MaxSleep = TimeSpan.FromHours(1);
    private static readonly TimeSpan WarningThreshold = TimeSpan.FromHours(1);

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var maxMinutes = config.GetValue("EXPIRY_CHECK_MAX_MINUTES", 60);
        var maxSleep = TimeSpan.FromMinutes(maxMinutes);

        logger.LogInformation("ExpiryWorker started, max sleep {Max}m", maxMinutes);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                var nextWake = await CheckExpiry(stoppingToken);
                var delay = nextWake is not null
                    ? Min(nextWake.Value - DateTime.UtcNow, maxSleep)
                    : maxSleep;

                if (delay < TimeSpan.FromSeconds(10))
                    delay = TimeSpan.FromSeconds(10);

                logger.LogDebug("ExpiryWorker sleeping {Delay}", delay);
                await Task.Delay(delay, stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Error checking VM expiry");
                await Task.Delay(TimeSpan.FromMinutes(1), stoppingToken);
            }
        }
    }

    private async Task<DateTime?> CheckExpiry(CancellationToken ct)
    {
        var now = DateTime.UtcNow;
        var vms = db.Vms.FindAll()
            .Where(v => v.ExpiresAt.HasValue && v.State is not VmState.Deleting)
            .ToList();

        if (vms.Count == 0) return null;

        DateTime? soonest = null;

        foreach (var vm in vms)
        {
            var remaining = vm.ExpiresAt!.Value - now;

            if (remaining <= TimeSpan.Zero)
            {
                logger.LogInformation("VM {Name} expired, dismissing", vm.Name);

                vm.State = VmState.Deleting;
                db.Vms.Update(vm);

                await kubeVirt.Delete(vm);

                db.Vms.Delete(vm.Id);
                db.Audit.Insert(new AuditEntry
                {
                    Action = "vm.expired",
                    ResourceId = vm.Id.ToString(),
                    Detail = $"Auto-dismissed {vm.Name} (expired)"
                });

                await notifier.Publish(new VmStateEvent(vm.Id, VmState.Deleting));
            }
            else
            {
                if (remaining <= WarningThreshold)
                {
                    await notifier.Publish(new VmStateEvent(vm.Id, vm.State, $"Expires in {(int)remaining.TotalMinutes}m"));
                }

                if (soonest is null || vm.ExpiresAt!.Value < soonest)
                    soonest = vm.ExpiresAt!.Value;
            }
        }

        return soonest;
    }

    private static TimeSpan Min(TimeSpan a, TimeSpan b) => a < b ? a : b;
}
