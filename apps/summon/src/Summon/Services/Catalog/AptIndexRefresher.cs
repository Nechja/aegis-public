using System.IO.Compression;

namespace Summon.Services.Catalog;

public sealed class AptIndexRefresher(
    AptIndex index,
    IHttpClientFactory httpFactory,
    IConfiguration config,
    ILogger<AptIndexRefresher> logger) : BackgroundService
{
    private static readonly TimeSpan RefreshEvery = TimeSpan.FromHours(24);
    private static readonly TimeSpan RetryAfterFailure = TimeSpan.FromMinutes(30);

    protected override async Task ExecuteAsync(CancellationToken ct)
    {
        var mirror = (config["APT_MIRROR"] ?? "http://archive.ubuntu.com/ubuntu").TrimEnd('/');
        var suite = config["APT_SUITE"] ?? "noble";
        var components = (config["APT_COMPONENTS"] ?? "main,universe")
            .Split(',', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);
        var arch = config["APT_ARCH"] ?? "amd64";

        while (!ct.IsCancellationRequested)
        {
            var ok = await TryRefresh(mirror, suite, components, arch, ct);
            try
            {
                await Task.Delay(ok ? RefreshEvery : RetryAfterFailure, ct);
            }
            catch (OperationCanceledException)
            {
                break;
            }
        }
    }

    private async Task<bool> TryRefresh(string mirror, string suite, string[] components, string arch, CancellationToken ct)
    {
        try
        {
            var byName = new Dictionary<string, AptIndexEntry>(capacity: 120_000, StringComparer.Ordinal);
            foreach (var component in components)
            {
                var added = await FetchInto(byName, mirror, suite, component, arch, ct);
                logger.LogInformation("Apt index: {Added} new entries from {Suite}/{Component}", added, suite, component);
            }
            index.Replace(byName.Values);
            logger.LogInformation("Apt index refreshed: {Count} entries", index.Count);
            return true;
        }
        catch (OperationCanceledException) when (ct.IsCancellationRequested)
        {
            return false;
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Apt index refresh failed; retrying in {Delay}", RetryAfterFailure);
            return false;
        }
    }

    private async Task<int> FetchInto(Dictionary<string, AptIndexEntry> sink, string mirror, string suite, string component, string arch, CancellationToken ct)
    {
        var url = $"{mirror}/dists/{suite}/{component}/binary-{arch}/Packages.gz";
        var http = httpFactory.CreateClient("apt-mirror");
        using var resp = await http.GetAsync(url, HttpCompletionOption.ResponseHeadersRead, ct);
        resp.EnsureSuccessStatusCode();

        await using var stream = await resp.Content.ReadAsStreamAsync(ct);
        await using var gz = new GZipStream(stream, CompressionMode.Decompress);
        using var reader = new StreamReader(gz);

        var added = 0;
        string? name = null;
        string? desc = null;
        string? section = null;

        while (!ct.IsCancellationRequested)
        {
            var line = await reader.ReadLineAsync(ct);
            if (line is null)
            {
                added += Flush(sink, ref name, ref desc, ref section);
                break;
            }
            if (line.Length == 0)
            {
                added += Flush(sink, ref name, ref desc, ref section);
                continue;
            }
            if (line[0] == ' ' || line[0] == '\t') continue;

            var colon = line.IndexOf(':');
            if (colon <= 0) continue;

            var key = line.AsSpan(0, colon);
            var value = line[(colon + 1)..].TrimStart();

            if (key.SequenceEqual("Package"))
                name = value;
            else if (key.SequenceEqual("Description") && desc is null)
                desc = value;
            else if (key.SequenceEqual("Section"))
                section = value;
        }

        return added;
    }

    private static int Flush(Dictionary<string, AptIndexEntry> sink, ref string? name, ref string? desc, ref string? section)
    {
        if (name is null)
        {
            desc = null; section = null;
            return 0;
        }

        var added = sink.TryAdd(name, new AptIndexEntry(name, desc, section)) ? 1 : 0;
        name = null; desc = null; section = null;
        return added;
    }
}
