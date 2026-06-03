using System.Text.Json;
using LiteDB;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;
using Summon.Models;

namespace Summon.Services.Data;

/// <summary>
/// Merges <c>Data/apt_snap_catalog.json</c> into LiteDB (insert-only by package <see cref="SoftwarePackage.Name"/>).
/// Rows are curated from common Ubuntu/Debian apt and snap names; regenerate the JSON offline if you need more.
/// </summary>
public static class AptSnapCatalogMerger
{
    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNameCaseInsensitive = true
    };

    public static void MergeFromFile(ILiteCollection<SoftwarePackage> packages, IHostEnvironment env, ILogger logger)
    {
        var path = Path.Combine(env.ContentRootPath, "Data", "apt_snap_catalog.json");
        if (!File.Exists(path))
        {
            logger.LogWarning("APT/snap catalog file not found at {Path}", path);
            return;
        }

        List<AptSnapCatalogDto>? rows;
        try
        {
            var json = File.ReadAllText(path);
            rows = System.Text.Json.JsonSerializer.Deserialize<List<AptSnapCatalogDto>>(json, JsonOpts);
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to read APT/snap catalog");
            return;
        }

        if (rows is null || rows.Count == 0)
            return;

        var inserted = 0;
        var updated = 0;
        foreach (var dto in rows)
        {
            var built = TryBuildPackage(dto);
            if (built is null)
            {
                logger.LogWarning("Skipping invalid catalog row {Name}", dto.Name);
                continue;
            }

            var existing = packages.FindOne(p => p.Name == built.Name);
            if (existing is not null)
            {
                // Upsert so edits to the JSON (fixed snippet, renamed displayName) land on redeploy.
                packages.Update(built with { Id = existing.Id });
                updated++;
                continue;
            }

            packages.Insert(built);
            inserted++;
        }

        if (inserted > 0 || updated > 0)
            logger.LogInformation("Merged APT/snap catalog: {Inserted} inserted, {Updated} updated", inserted, updated);
    }

    private static SoftwarePackage? TryBuildPackage(AptSnapCatalogDto dto)
    {
        var name = dto.Name?.Trim();
        if (string.IsNullOrEmpty(name))
            return null;

        var display = string.IsNullOrWhiteSpace(dto.DisplayName) ? name : dto.DisplayName.Trim();
        var category = string.IsNullOrWhiteSpace(dto.Category) ? "General" : dto.Category.Trim();
        var source = (dto.Source ?? "script").Trim().ToLowerInvariant();

        return source switch
        {
            PackageLinuxSource.Apt => BuildApt(name, display, category, dto),
            PackageLinuxSource.Snap => BuildSnap(name, display, category, dto),
            PackageLinuxSource.Script => BuildScript(name, display, category, dto),
            _ => null
        };
    }

    private static SoftwarePackage? BuildApt(string name, string display, string category, AptSnapCatalogDto dto)
    {
        var apt = (dto.AptPackage ?? dto.Name).Trim();
        if (apt.Length == 0) return null;
        // Omit per-package apt-get update so many parallel installs stay fast; cloud-init usually refreshes indexes once.
        var snippet =
            "export DEBIAN_FRONTEND=noninteractive && apt-get install -y " + apt;
        return new SoftwarePackage
        {
            Name = name,
            DisplayName = display,
            Category = category,
            LinuxSource = PackageLinuxSource.Apt,
            AptPackageName = apt,
            LinuxSnippet = snippet
        };
    }

    private static SoftwarePackage? BuildSnap(string name, string display, string category, AptSnapCatalogDto dto)
    {
        var snap = (dto.Snap ?? dto.Name).Trim();
        if (snap.Length == 0) return null;
        var classic = dto.Classic;
        var snippet = classic ? $"snap install {snap} --classic" : $"snap install {snap}";
        return new SoftwarePackage
        {
            Name = name,
            DisplayName = display,
            Category = category,
            LinuxSource = PackageLinuxSource.Snap,
            SnapName = snap,
            SnapClassic = classic,
            LinuxSnippet = snippet
        };
    }

    private static SoftwarePackage? BuildScript(string name, string display, string category, AptSnapCatalogDto dto)
    {
        var script = (dto.Script ?? string.Empty).Trim();
        if (script.Length == 0) return null;
        return new SoftwarePackage
        {
            Name = name,
            DisplayName = display,
            Category = category,
            LinuxSource = PackageLinuxSource.Script,
            LinuxSnippet = script
        };
    }

    private sealed class AptSnapCatalogDto
    {
        public string? Name { get; set; }
        public string? DisplayName { get; set; }
        public string? Category { get; set; }
        public string? Source { get; set; }
        public string? AptPackage { get; set; }
        public string? Snap { get; set; }
        public bool Classic { get; set; }
        public string? Script { get; set; }
    }
}
