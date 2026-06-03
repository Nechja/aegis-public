namespace Summon.Services;

public sealed record VmStateEvent(Guid VmId, string State, string? Progress = null);

public sealed class VmStateNotifier
{
    private readonly List<Func<VmStateEvent, Task>> _subscribers = [];
    private readonly Lock _lock = new();

    public IDisposable Subscribe(Func<VmStateEvent, Task> handler)
    {
        lock (_lock) _subscribers.Add(handler);
        return new Subscription(this, handler);
    }

    public async Task Publish(VmStateEvent evt)
    {
        Func<VmStateEvent, Task>[] handlers;
        lock (_lock) handlers = [.. _subscribers];

        foreach (var handler in handlers)
        {
            try { await handler(evt); }
            catch { /* don't let one subscriber break others */ }
        }
    }

    private void Unsubscribe(Func<VmStateEvent, Task> handler)
    {
        lock (_lock) _subscribers.Remove(handler);
    }

    private sealed class Subscription(VmStateNotifier notifier, Func<VmStateEvent, Task> handler) : IDisposable
    {
        public void Dispose() => notifier.Unsubscribe(handler);
    }
}
