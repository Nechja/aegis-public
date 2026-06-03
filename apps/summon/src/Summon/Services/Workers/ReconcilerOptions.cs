namespace Summon.Services.Workers;

public sealed class ReconcilerOptions
{
    /// <summary>Cronos expression (with seconds). Default: every 15 seconds.</summary>
    public string Schedule { get; init; } = "*/15 * * * * *";

    /// <summary>How long a DB-only record must have existed before the reconciler is allowed to prune it.</summary>
    public TimeSpan GracePeriod { get; init; } = TimeSpan.FromMinutes(1);

    /// <summary>Namespace to reconcile. Overrides <see cref="KubeClientFactory.VmNamespace"/> for the reconciler when set.</summary>
    public string Namespace { get; init; } = "vms";
}
