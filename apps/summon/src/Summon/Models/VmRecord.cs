namespace Summon.Models;

public record VmRecord
{
    public Guid Id { get; init; } = Guid.NewGuid();
    public required string Name { get; init; }
    public required string TemplateName { get; init; }
    /// <summary>Copied from the template at summon time (<see cref="TemplateOsFamilies"/>).</summary>
    public string OsFamily { get; init; } = TemplateOsFamilies.Linux;
    public string InstanceType { get; init; } = "medium";
    public int DiskGb { get; init; } = 30;
    public string Arch { get; init; } = "amd64";
    public string State { get; set; } = VmState.Creating;
    public string KubeNamespace { get; init; } = "vms";
    public string KubeVmName { get; init; } = string.Empty;
    public string TailscaleHostname { get; init; } = string.Empty;
    public string TailscaleIp { get; set; } = string.Empty;
    public required List<string> Packages { get; init; }
    /// <summary>Optional install ledger (newer summons). When null, only <see cref="Packages"/> keys are known.</summary>
    public List<PackageInstallEntry>? SoftwareInstalls { get; set; }
    public string CloudInit { get; init; } = string.Empty;
    public DateTime CreatedAt { get; init; } = DateTime.UtcNow;
    public DateTime? ExpiresAt { get; init; }
    /// <summary>Latest <c>apt list --upgradable</c> / <c>snap list</c> style snapshot from the guest agent.</summary>
    public string? PackageManagerSnapshot { get; set; }
    public DateTime? PackageManagerSnapshotAt { get; set; }

    /// <summary>Structured pending-update counts from the most recent inventory probe.</summary>
    public int? PendingAptUpdates { get; set; }
    public int? PendingSecurityUpdates { get; set; }
    public int? PendingSnapRefreshes { get; set; }
    /// <summary>Timestamp of the last inventory.refresh (apt-get update) — null if only cached probes have run.</summary>
    public DateTime? LastIndexRefreshedAt { get; set; }

    /// <summary>Most recent facts probe from the guest agent (CPU / memory / disk / uptime / kernel / primary IP).</summary>
    public TelemetrySnapshot? Telemetry { get; set; }

    /// <summary>Agent version reported on <c>agent.ready</c>. Null until the guest first connects.</summary>
    public string? AgentVersion { get; set; }

    /// <summary>Structured list of currently installed apps from the guest (apt + snap). Null until the first <c>apps.list</c> probe returns.</summary>
    public List<InstalledApp>? InstalledApps { get; set; }
    public DateTime? InstalledAppsCapturedAt { get; set; }

    /// <summary>True if this record was adopted by the reconciler from an out-of-band VM in the cluster.</summary>
    public bool ExternalOrigin { get; set; } = false;
}

public static class VmState
{
    public const string Creating = "creating";
    public const string Importing = "importing";
    public const string Starting = "starting";
    public const string Running = "running";
    public const string Stopped = "stopped";
    public const string Failed = "failed";
    public const string Deleting = "deleting";
}
