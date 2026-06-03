namespace Summon.Models;

/// <summary>How <see cref="SoftwarePackage.LinuxSnippet"/> was produced (for UI and filters).</summary>
public static class PackageLinuxSource
{
    public const string Apt = "apt";
    public const string Snap = "snap";
    public const string Script = "script";
}
