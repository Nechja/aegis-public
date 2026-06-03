using System.Net.Http.Json;

namespace Summon.Services.Catalog;

public sealed class SnapStoreClient(HttpClient http, ILogger<SnapStoreClient> logger)
{
    public async Task<IReadOnlyList<CatalogSearchResult>> Search(string term, int limit = 10, CancellationToken ct = default)
    {
        if (string.IsNullOrWhiteSpace(term)) return [];

        var url = $"/v2/snaps/find?q={Uri.EscapeDataString(term.Trim())}&fields=title,summary";
        try
        {
            using var resp = await http.GetAsync(url, ct);
            if (!resp.IsSuccessStatusCode)
            {
                logger.LogDebug("Snap Store returned {Status} for {Term}", (int)resp.StatusCode, term);
                return [];
            }
            var body = await resp.Content.ReadFromJsonAsync<SnapFindResponse>(cancellationToken: ct);
            if (body?.Results is null) return [];

            return body.Results
                .Where(r => !string.IsNullOrWhiteSpace(r.Name))
                .Take(limit)
                .Select(r => new CatalogSearchResult(
                    "snap",
                    r.Name!,
                    string.IsNullOrWhiteSpace(r.Snap?.Title) ? r.Name! : r.Snap!.Title!,
                    r.Snap?.Summary,
                    null))
                .ToList();
        }
        catch (OperationCanceledException)
        {
            return [];
        }
        catch (Exception ex)
        {
            logger.LogWarning(ex, "Snap Store search failed for {Term}", term);
            return [];
        }
    }

    private sealed class SnapFindResponse
    {
        public List<SnapResult>? Results { get; set; }
    }

    private sealed class SnapResult
    {
        public string? Name { get; set; }
        public SnapInfo? Snap { get; set; }
    }

    private sealed class SnapInfo
    {
        public string? Title { get; set; }
        public string? Summary { get; set; }
    }
}
