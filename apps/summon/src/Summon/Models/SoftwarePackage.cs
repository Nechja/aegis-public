namespace Summon.Models;

public record SoftwarePackage
{
    public Guid Id { get; init; } = Guid.NewGuid();
    public required string Name { get; init; }
    public required string DisplayName { get; init; }
    /// <summary>Used to group rows on the summon form (e.g. Languages, DevOps).</summary>
    public string Category { get; init; } = "General";
    /// <summary><see cref="PackageLinuxSource"/> — apt/snap rows come from the merged catalog JSON.</summary>
    public string LinuxSource { get; init; } = PackageLinuxSource.Script;
    /// <summary>Debian package name when <see cref="LinuxSource"/> is apt.</summary>
    public string? AptPackageName { get; init; }
    /// <summary>Snap name when <see cref="LinuxSource"/> is snap.</summary>
    public string? SnapName { get; init; }
    public bool SnapClassic { get; init; }
    public string LinuxSnippet { get; init; } = string.Empty;
    public string WindowsSnippet { get; init; } = string.Empty;
    public string Arch { get; init; } = "both";
}
