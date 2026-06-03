using LiteDB;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;
using Summon.Models;

namespace Summon.Services.Data;

public sealed class SummonDb : IHostedService, IDisposable
{
    private readonly LiteDatabase _db;
    private readonly ILogger<SummonDb> _logger;

    public SummonDb(IConfiguration config, IHostEnvironment env, ILogger<SummonDb> logger)
    {
        _logger = logger;
        var path = config["LITEDB_PATH"] ?? "/data/summon.db";
        _db = new LiteDatabase(path);

        Vms = _db.GetCollection<VmRecord>("vms");
        Vms.EnsureIndex(x => x.Name);
        Vms.EnsureIndex(x => x.State);

        Templates = _db.GetCollection<VmTemplate>("templates");
        Templates.EnsureIndex(x => x.Name);

        Packages = _db.GetCollection<SoftwarePackage>("packages");
        Packages.EnsureIndex(x => x.Name);

        Audit = _db.GetCollection<AuditEntry>("audit");
        Audit.EnsureIndex(x => x.Timestamp);

        Operations = _db.GetCollection<FleetOperation>("operations");
        Operations.EnsureIndex(x => x.StartedAt);

        Identities = _db.GetCollection<Identity>("identities");

        EnsureCatalog(env, logger);
    }

    public ILiteCollection<VmRecord> Vms { get; }
    public ILiteCollection<VmTemplate> Templates { get; }
    public ILiteCollection<SoftwarePackage> Packages { get; }
    public ILiteCollection<AuditEntry> Audit { get; }
    public ILiteCollection<FleetOperation> Operations { get; }
    public ILiteCollection<Identity> Identities { get; }

    private void EnsureCatalog(IHostEnvironment env, ILogger<SummonDb> logger)
    {
        // Older DB rows may lack OsFamily; treat as Linux so filtering and KubeVirt stay consistent.
        foreach (var existing in Templates.FindAll().ToList())
        {
            if (!string.IsNullOrWhiteSpace(existing.OsFamily))
                continue;
            Templates.Update(existing with
            {
                OsFamily = TemplateOsFamilies.Linux,
                ProvisioningEnabled = true
            });
        }

        // Add new catalog entries (e.g. Kali, Windows) without clobbering customized BaseImage on existing templates.
        foreach (var desired in BuiltinTemplates())
        {
            if (Templates.FindOne(t => t.Name == desired.Name) is not null)
                continue;
            Templates.Insert(desired);
        }

        if (Packages.Count() == 0)
        {
            Packages.InsertBulk([
                new SoftwarePackage
                {
                    Name = "docker", DisplayName = "Docker", Category = "DevOps",
                    LinuxSource = PackageLinuxSource.Script,
                    LinuxSnippet = "curl -fsSL https://get.docker.com | sh"
                },
                new SoftwarePackage
                {
                    Name = "go", DisplayName = "Go 1.22", Category = "Languages",
                    LinuxSource = PackageLinuxSource.Snap, SnapName = "go", SnapClassic = true,
                    LinuxSnippet = "snap install go --classic"
                },
                new SoftwarePackage
                {
                    Name = "node22", DisplayName = "Node.js 22", Category = "Languages",
                    LinuxSource = PackageLinuxSource.Script,
                    LinuxSnippet = "curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && apt-get install -y nodejs"
                },
                new SoftwarePackage
                {
                    Name = "python3", DisplayName = "Python 3", Category = "Languages",
                    LinuxSource = PackageLinuxSource.Apt, AptPackageName = "python3 python3-pip python3-venv",
                    LinuxSnippet = "export DEBIAN_FRONTEND=noninteractive && apt-get install -y python3 python3-pip python3-venv"
                },
                new SoftwarePackage
                {
                    Name = "git", DisplayName = "Git", Category = "Essentials",
                    LinuxSource = PackageLinuxSource.Apt, AptPackageName = "git",
                    LinuxSnippet = "export DEBIAN_FRONTEND=noninteractive && apt-get install -y git"
                },
                new SoftwarePackage
                {
                    Name = "vim", DisplayName = "Vim", Category = "Editors",
                    LinuxSource = PackageLinuxSource.Apt, AptPackageName = "vim",
                    LinuxSnippet = "export DEBIAN_FRONTEND=noninteractive && apt-get install -y vim"
                },
                new SoftwarePackage
                {
                    Name = "kubectl", DisplayName = "kubectl", Category = "DevOps",
                    LinuxSource = PackageLinuxSource.Snap, SnapName = "kubectl", SnapClassic = true,
                    LinuxSnippet = "snap install kubectl --classic"
                },
                new SoftwarePackage
                {
                    Name = "rust", DisplayName = "Rust", Category = "Languages",
                    LinuxSource = PackageLinuxSource.Script,
                    LinuxSnippet = "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y"
                },
            ]);
        }

        AptSnapCatalogMerger.MergeFromFile(Packages, env, logger);
    }

    /// <summary>Built-in rows merged into LiteDB on startup (swap <see cref="VmTemplate.BaseImage"/> when your images land in MinIO).</summary>
    private static IEnumerable<VmTemplate> BuiltinTemplates()
    {
        var linuxGuest = """
            #cloud-config
            package_update: true
            packages:
              - qemu-guest-agent
            runcmd:
              - systemctl enable --now qemu-guest-agent
            """;

        yield return new VmTemplate
        {
            Name = "Ubuntu 24.04",
            BaseImage = "noble-server-cloudimg-amd64.img",
            OsFamily = TemplateOsFamilies.Linux,
            ProvisioningEnabled = true,
            Arch = "amd64",
            MinCpu = 1,
            MinMemoryGb = 1,
            BaseCloudInit = linuxGuest
        };

        yield return new VmTemplate
        {
            Name = "Debian 12",
            BaseImage = "debian-12-genericcloud-amd64.qcow2",
            OsFamily = TemplateOsFamilies.Linux,
            ProvisioningEnabled = true,
            Arch = "amd64",
            MinCpu = 1,
            MinMemoryGb = 1,
            BaseCloudInit = linuxGuest
        };

        // Kali: same Linux + cloud-init + summon-agent path as Debian/Ubuntu; replace BaseImage when uploaded.
        yield return new VmTemplate
        {
            Name = "Kali Linux (rolling)",
            BaseImage = "kali-linux-genericcloud-amd64.qcow2",
            OsFamily = TemplateOsFamilies.Linux,
            ProvisioningEnabled = true,
            Arch = "amd64",
            MinCpu = 2,
            MinMemoryGb = 2,
            BaseCloudInit = linuxGuest
        };

        // Windows: catalogued for UI / planning; enable when sysprep or cloudbase-init + KubeVirt volumes are implemented.
        yield return new VmTemplate
        {
            Name = "Windows Server 2022",
            BaseImage = "windows-server-2022-datacenter-core.qcow2",
            OsFamily = TemplateOsFamilies.Windows,
            ProvisioningEnabled = false,
            Arch = "amd64",
            MinCpu = 2,
            MinMemoryGb = 4,
            MinDiskGb = 40,
            BaseCloudInit = string.Empty
        };
    }

    public Task StartAsync(CancellationToken cancellationToken) => Task.CompletedTask;

    /// <summary>
    /// Force a LiteDB checkpoint on shutdown so the WAL log is empty when the process exits.
    /// Without this, an interrupted checkpoint can leave the main file with inconsistent pages —
    /// the failure mode that produced the "page type must be index page" corruption on 2026-04-29.
    /// </summary>
    public Task StopAsync(CancellationToken cancellationToken)
    {
        try
        {
            _logger.LogInformation("Checkpointing LiteDB before shutdown");
            _db.Checkpoint();
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "LiteDB checkpoint on shutdown failed");
        }
        return Task.CompletedTask;
    }

    public void Dispose() => _db.Dispose();
}
