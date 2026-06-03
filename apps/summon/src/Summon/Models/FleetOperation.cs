namespace Summon.Models;

public static class FleetOperationKind
{
    public const string Upgrade = "upgrade";
    public const string Install = "install";
    public const string Uninstall = "uninstall";
    public const string AppsList = "apps.list";
    public const string Exec = "exec";
}

public static class FleetOperationTargetPhase
{
    public const string Pending = "pending";
    public const string Running = "running";
    public const string Done = "done";
    public const string Failed = "failed";
    public const string Abandoned = "abandoned";
}

public sealed class FleetOperationTarget
{
    public Guid VmId { get; set; }
    public string VmName { get; set; } = string.Empty;
    public string Phase { get; set; } = FleetOperationTargetPhase.Pending;
    public string? Message { get; set; }
    public int? ExitCode { get; set; }
    /// <summary>Tail of the agent's stdout/stderr captured on terminal events; null while pending/running.</summary>
    public string? Output { get; set; }
    public DateTime UpdatedAt { get; set; } = DateTime.UtcNow;
}

public sealed class FleetOperation
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public string Kind { get; set; } = string.Empty;
    /// <summary>Human label (e.g. "upgrade", "install htop", "uninstall vim, curl"). Used in the ops popover.</summary>
    public string Summary { get; set; } = string.Empty;
    public DateTime StartedAt { get; set; } = DateTime.UtcNow;
    public DateTime? CompletedAt { get; set; }
    public List<FleetOperationTarget> Targets { get; set; } = [];
}
