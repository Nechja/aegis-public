using System.Collections.Immutable;

namespace Summon.Services.Catalog;

public sealed record AptIndexEntry(string Name, string? Description, string? Section);

public sealed class AptIndex
{
    private ImmutableArray<AptIndexEntry> _entries = [];

    public int Count => _entries.Length;
    public DateTime? LastRefreshed { get; private set; }

    public void Replace(IEnumerable<AptIndexEntry> entries)
    {
        _entries = entries.ToImmutableArray();
        LastRefreshed = DateTime.UtcNow;
    }

    public IReadOnlyList<CatalogSearchResult> Search(string term, int limit = 10)
    {
        if (string.IsNullOrWhiteSpace(term) || _entries.Length == 0) return [];
        var q = term.Trim();

        var exact = new List<AptIndexEntry>();
        var prefix = new List<AptIndexEntry>();
        var contains = new List<AptIndexEntry>();

        foreach (var e in _entries)
        {
            if (e.Name.Equals(q, StringComparison.OrdinalIgnoreCase))
                exact.Add(e);
            else if (e.Name.StartsWith(q, StringComparison.OrdinalIgnoreCase))
                prefix.Add(e);
            else if (e.Name.Contains(q, StringComparison.OrdinalIgnoreCase))
                contains.Add(e);

            if (exact.Count + prefix.Count + contains.Count >= limit * 4)
                break;
        }

        return exact.Concat(prefix).Concat(contains)
            .Take(limit)
            .Select(e => new CatalogSearchResult("apt", e.Name, e.Name, e.Description, e.Section))
            .ToList();
    }
}
