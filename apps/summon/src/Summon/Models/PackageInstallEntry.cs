namespace Summon.Models;

/// <summary>Per-package install state persisted on <see cref="VmRecord"/> and updated from agent reports.</summary>
public sealed class PackageInstallEntry
{
    public string PackageKey { get; set; } = string.Empty;
    public string DisplayName { get; set; } = string.Empty;
    public string ScriptSnippet { get; set; } = string.Empty;
    public string Phase { get; set; } = PackageInstallPhase.Pending;
    public string? LastOutput { get; set; }
    public int? ExitCode { get; set; }
    public DateTime? UpdatedAt { get; set; }
}
