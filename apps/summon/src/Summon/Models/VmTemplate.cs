namespace Summon.Models;

public record VmTemplate
{
    public Guid Id { get; init; } = Guid.NewGuid();
    public required string Name { get; init; }
    public required string BaseImage { get; init; }
    /// <summary><see cref="TemplateOsFamilies"/> — drives userdata, agent installs, and UI.</summary>
    public string OsFamily { get; init; } = TemplateOsFamilies.Linux;
    /// <summary>When false, creation is blocked until Windows userdata / sysprep (or similar) is wired in KubeVirt.</summary>
    public bool ProvisioningEnabled { get; init; } = true;
    public string Arch { get; init; } = "amd64";
    public int MinCpu { get; init; } = 1;
    public int MinMemoryGb { get; init; } = 1;
    /// <summary>Floor for the root DataVolume. Effective disk size is max(this, instance-type default).</summary>
    public int MinDiskGb { get; init; } = 10;
    public string BaseCloudInit { get; init; } = string.Empty;
}
