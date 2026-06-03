namespace Summon.Services.Catalog;

public sealed record CatalogSearchResult(
    string Source,
    string Name,
    string DisplayName,
    string? Description,
    string? Section);
