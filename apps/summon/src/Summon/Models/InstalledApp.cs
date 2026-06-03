namespace Summon.Models;

/// <summary>A structured row in the guest's "currently installed" list, collected by <c>apps.list</c>.</summary>
public sealed class InstalledApp
{
    /// <summary><c>apt</c> or <c>snap</c>.</summary>
    public string Source { get; set; } = string.Empty;
    public string Name { get; set; } = string.Empty;
    public string Version { get; set; } = string.Empty;
    /// <summary>Optional dpkg status abbreviation (e.g. <c>ii</c>). Only populated for apt rows.</summary>
    public string? Status { get; set; }
}
