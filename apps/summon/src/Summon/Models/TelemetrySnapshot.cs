namespace Summon.Models;

/// <summary>Structured facts captured by <c>summon-agent</c> via a <c>facts.done</c> report.</summary>
public sealed class TelemetrySnapshot
{
    public string? Kernel { get; set; }
    public string? OsPretty { get; set; }
    public long UptimeSec { get; set; }
    public double CpuPercent { get; set; }
    public int CpuCount { get; set; }
    public double LoadAvg1 { get; set; }
    public long MemTotalKb { get; set; }
    public long MemUsedKb { get; set; }
    public double MemPercent { get; set; }
    public long DiskTotalKb { get; set; }
    public long DiskUsedKb { get; set; }
    public double DiskPercent { get; set; }
    public string? PrimaryIp { get; set; }
    public string? AgentVersion { get; set; }
    public DateTime CapturedAt { get; set; } = DateTime.UtcNow;
}
