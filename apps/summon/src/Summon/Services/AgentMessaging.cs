using System.Text.Json;
using RabbitMQ.Client;
using Summon.Models;

namespace Summon.Services;

public static class AgentJson
{
    public static readonly JsonSerializerOptions Serializer = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull
    };
}

public sealed record InstallCommandItem(string Id, string Label, string Script);

public sealed record InstallAgentMessage(string Type, List<InstallCommandItem> Items, string? OperationId = null);

public sealed record ExecCommand(string Type, string Script, string? OperationId = null);

/// <summary>Single-field commands understood by summon-agent (e.g. inventory probe / apps.list / upgrade).</summary>
public sealed record TypedAgentCommand(string Type, string? OperationId = null);

public sealed record AgentFacts(
    string? Kernel = null,
    string? OsPretty = null,
    long UptimeSec = 0,
    double CpuPercent = 0,
    int CpuCount = 0,
    double LoadAvg1 = 0,
    long MemTotalKB = 0,
    long MemUsedKB = 0,
    double MemPercent = 0,
    long DiskTotalKB = 0,
    long DiskUsedKB = 0,
    double DiskPercent = 0,
    string? PrimaryIp = null,
    string? AgentVersion = null);

public sealed record AgentPending(
    int AptUpgradable = 0,
    int AptSecurity = 0,
    int SnapRefreshable = 0,
    bool IndexRefreshed = false);

public sealed record AgentReport(
    string VmName,
    string Type,
    string Status,
    string? Package = null,
    string? PackageId = null,
    string? Label = null,
    string? Output = null,
    int ExitCode = 0,
    string? Timestamp = null,
    string? AgentVersion = null,
    AgentFacts? Facts = null,
    List<InstalledApp>? Apps = null,
    AgentPending? Pending = null,
    string? OperationId = null);

public sealed class AgentMessaging : IHostedService, IAsyncDisposable
{
    private static readonly TimeSpan InitialBackoff = TimeSpan.FromSeconds(2);
    private static readonly TimeSpan MaxBackoff = TimeSpan.FromSeconds(30);

    private readonly ILogger<AgentMessaging> _logger;
    private readonly ConnectionFactory _factory;
    private readonly string _host;
    private readonly int _port;

    private IConnection? _connection;
    private IChannel? _channel;
    private volatile bool _connected;

    private CancellationTokenSource? _cts;
    private Task? _connectLoop;

    public AgentMessaging(IConfiguration config, ILogger<AgentMessaging> logger)
    {
        _logger = logger;

        _host = config["RABBITMQ_HOST"] ?? "rabbitmq.rabbitmq.svc.cluster.local";
        _port = int.Parse(config["RABBITMQ_PORT"] ?? "5672");
        var user = config["RABBITMQ_USER"] ?? "aegis";
        var pass = config["RABBITMQ_PASS"] ?? "guest";

        _factory = new ConnectionFactory
        {
            HostName = _host,
            Port = _port,
            UserName = user,
            Password = pass
        };
    }

    public Task StartAsync(CancellationToken cancellationToken)
    {
        _cts = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        _connectLoop = Task.Run(() => ConnectWithRetryAsync(_cts.Token));
        return Task.CompletedTask;
    }

    public async Task StopAsync(CancellationToken cancellationToken)
    {
        _cts?.Cancel();
        if (_connectLoop is not null)
        {
            try { await _connectLoop.WaitAsync(cancellationToken); } catch { }
        }
    }

    private async Task ConnectWithRetryAsync(CancellationToken ct)
    {
        var backoff = InitialBackoff;
        while (!ct.IsCancellationRequested)
        {
            try
            {
                var connection = await _factory.CreateConnectionAsync(ct);
                var channel = await connection.CreateChannelAsync(cancellationToken: ct);

                connection.ConnectionShutdownAsync += OnConnectionShutdown;

                _connection = connection;
                _channel = channel;
                _connected = true;
                _logger.LogInformation("Connected to RabbitMQ at {Host}:{Port}", _host, _port);
                return;
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested)
            {
                return;
            }
            catch (Exception ex)
            {
                _logger.LogWarning(ex, "RabbitMQ connect failed; retrying in {Backoff}", backoff);
                try { await Task.Delay(backoff, ct); } catch (OperationCanceledException) { return; }
                backoff = TimeSpan.FromMilliseconds(Math.Min(MaxBackoff.TotalMilliseconds, backoff.TotalMilliseconds * 2));
            }
        }
    }

    private Task OnConnectionShutdown(object? sender, RabbitMQ.Client.Events.ShutdownEventArgs args)
    {
        _connected = false;
        _channel = null;
        _connection = null;
        _logger.LogWarning("RabbitMQ connection lost ({Reason}); reconnecting in background", args.ReplyText);

        var ct = _cts?.Token ?? CancellationToken.None;
        if (!ct.IsCancellationRequested)
        {
            _connectLoop = Task.Run(() => ConnectWithRetryAsync(ct));
        }
        return Task.CompletedTask;
    }

    public async Task SendInstallItems(string vmName, IReadOnlyList<InstallCommandItem> items, string? operationId = null)
    {
        if (!_connected || _channel is null || items.Count == 0) return;

        var queue = $"summon.agent.{vmName}";
        await _channel.QueueDeclareAsync(queue, durable: true, exclusive: false, autoDelete: false);

        var cmd = new InstallAgentMessage("install", [.. items], operationId);
        var body = JsonSerializer.SerializeToUtf8Bytes(cmd, AgentJson.Serializer);

        await _channel.BasicPublishAsync("", queue, body);
        _logger.LogInformation("Sent install command to {Queue} with {Count} items", queue, items.Count);
    }

    public async Task SendUninstall(string vmName, IReadOnlyList<InstallCommandItem> items, string? operationId = null)
    {
        if (!_connected || _channel is null || items.Count == 0) return;

        var queue = $"summon.agent.{vmName}";
        await _channel.QueueDeclareAsync(queue, durable: true, exclusive: false, autoDelete: false);

        var cmd = new InstallAgentMessage("uninstall", [.. items], operationId);
        var body = JsonSerializer.SerializeToUtf8Bytes(cmd, AgentJson.Serializer);

        await _channel.BasicPublishAsync("", queue, body);
        _logger.LogInformation("Sent uninstall command to {Queue} with {Count} items", queue, items.Count);
    }

    public async Task SendUpgrade(string vmName, string? operationId = null)
    {
        if (!_connected || _channel is null) return;

        var queue = $"summon.agent.{vmName}";
        await _channel.QueueDeclareAsync(queue, durable: true, exclusive: false, autoDelete: false);

        var cmd = new TypedAgentCommand("upgrade", operationId);
        var body = JsonSerializer.SerializeToUtf8Bytes(cmd, AgentJson.Serializer);

        await _channel.BasicPublishAsync("", queue, body);
        _logger.LogInformation("Sent upgrade command to {Queue}", queue);
    }

    public async Task SendAppsList(string vmName, string? operationId = null)
    {
        if (!_connected || _channel is null) return;

        var queue = $"summon.agent.{vmName}";
        await _channel.QueueDeclareAsync(queue, durable: true, exclusive: false, autoDelete: false);

        var cmd = new TypedAgentCommand("apps.list", operationId);
        var body = JsonSerializer.SerializeToUtf8Bytes(cmd, AgentJson.Serializer);

        await _channel.BasicPublishAsync("", queue, body);
        _logger.LogInformation("Sent apps.list probe to {Queue}", queue);
    }

    public async Task SendExecCommand(string vmName, string script, string? operationId = null)
    {
        if (!_connected || _channel is null) return;

        var queue = $"summon.agent.{vmName}";
        await _channel.QueueDeclareAsync(queue, durable: true, exclusive: false, autoDelete: false);

        var cmd = new ExecCommand("exec", script, operationId);
        var body = JsonSerializer.SerializeToUtf8Bytes(cmd, AgentJson.Serializer);

        await _channel.BasicPublishAsync("", queue, body);
    }

    /// <summary>Asks the guest to capture apt upgradable list and snap list (Ubuntu/Debian).</summary>
    public async Task SendInventoryProbe(string vmName)
    {
        if (!_connected || _channel is null) return;

        var queue = $"summon.agent.{vmName}";
        await _channel.QueueDeclareAsync(queue, durable: true, exclusive: false, autoDelete: false);

        var cmd = new TypedAgentCommand("inventory");
        var body = JsonSerializer.SerializeToUtf8Bytes(cmd, AgentJson.Serializer);

        await _channel.BasicPublishAsync("", queue, body);
        _logger.LogInformation("Sent inventory probe to {Queue}", queue);
    }

    /// <summary>
    /// Refreshes the apt index (apt-get update) on the guest before snapshotting upgradable
    /// packages. Heavier than <see cref="SendInventoryProbe"/>; intended for slow cadence
    /// (every several hours), so pending-update counts don't drift indefinitely.
    /// </summary>
    public async Task SendInventoryRefresh(string vmName)
    {
        if (!_connected || _channel is null) return;

        var queue = $"summon.agent.{vmName}";
        await _channel.QueueDeclareAsync(queue, durable: true, exclusive: false, autoDelete: false);

        var cmd = new TypedAgentCommand("inventory.refresh");
        var body = JsonSerializer.SerializeToUtf8Bytes(cmd, AgentJson.Serializer);

        await _channel.BasicPublishAsync("", queue, body);
        _logger.LogInformation("Sent inventory.refresh to {Queue}", queue);
    }

    /// <summary>Asks the guest agent to capture CPU/mem/disk/uptime/kernel/primary-IP facts.</summary>
    public async Task SendFactsProbe(string vmName)
    {
        if (!_connected || _channel is null) return;

        var queue = $"summon.agent.{vmName}";
        await _channel.QueueDeclareAsync(queue, durable: true, exclusive: false, autoDelete: false);

        var cmd = new TypedAgentCommand("facts");
        var body = JsonSerializer.SerializeToUtf8Bytes(cmd, AgentJson.Serializer);

        await _channel.BasicPublishAsync("", queue, body);
        _logger.LogInformation("Sent facts probe to {Queue}", queue);
    }

    public async ValueTask DisposeAsync()
    {
        if (_channel is not null) await _channel.CloseAsync();
        if (_connection is not null) await _connection.CloseAsync();
    }
}
