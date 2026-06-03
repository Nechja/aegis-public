namespace Summon.Models;

public record AuditEntry
{
    public Guid Id { get; init; } = Guid.NewGuid();
    public required string Action { get; init; }
    public required string ResourceId { get; init; }
    public string Detail { get; init; } = string.Empty;
    public DateTime Timestamp { get; init; } = DateTime.UtcNow;
}
