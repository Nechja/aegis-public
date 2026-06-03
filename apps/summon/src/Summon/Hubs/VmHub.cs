using Microsoft.AspNetCore.SignalR;

namespace Summon.Hubs;

public sealed class VmHub : Hub
{
    public override Task OnConnectedAsync()
    {
        Groups.AddToGroupAsync(Context.ConnectionId, "vm-updates");
        return base.OnConnectedAsync();
    }
}
