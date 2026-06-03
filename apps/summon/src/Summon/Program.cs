using MudBlazor.Services;
using Summon.Components;
using Summon.Hubs;
using Summon.Services;
using Summon.Services.Catalog;
using Summon.Services.Data;
using Summon.Services.Workers;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddRazorComponents()
    .AddInteractiveServerComponents();

builder.Services.AddMudServices();
builder.Services.AddSignalR();
builder.Services.Configure<Microsoft.AspNetCore.Components.Server.CircuitOptions>(o => o.DetailedErrors = true);

// Data
builder.Services.AddSingleton<SummonDb>();
// Register as hosted service so the host invokes StopAsync (Checkpoint + flush) on SIGTERM.
builder.Services.AddHostedService(sp => sp.GetRequiredService<SummonDb>());

// Services
builder.Services.AddSingleton<KubeClientFactory>();
builder.Services.AddSingleton<CloudInitRenderer>();
builder.Services.AddSingleton<KubeVirtService>();
builder.Services.AddSingleton<VmStateNotifier>();
builder.Services.AddSingleton<FleetSnapshot>();
builder.Services.AddSingleton<AgentMessaging>();
builder.Services.AddHostedService(sp => sp.GetRequiredService<AgentMessaging>());
builder.Services.AddSingleton<FleetOperationService>();
builder.Services.AddSingleton<IdentityService>();

// Catalog (live search)
builder.Services.AddSingleton<AptIndex>();
builder.Services.AddHttpClient("apt-mirror", c =>
{
    c.Timeout = TimeSpan.FromMinutes(5);
    c.DefaultRequestHeaders.UserAgent.ParseAdd("summon-catalog/0.1");
});
builder.Services.AddHttpClient<SnapStoreClient>(c =>
{
    c.BaseAddress = new Uri("https://api.snapcraft.io");
    c.Timeout = TimeSpan.FromSeconds(8);
    c.DefaultRequestHeaders.Add("Snap-Device-Series", "16");
    c.DefaultRequestHeaders.UserAgent.ParseAdd("summon-catalog/0.1");
});

// Background workers
builder.Services.Configure<ReconcilerOptions>(builder.Configuration.GetSection("Reconciler"));
builder.Services.AddHostedService<KubeWatcher>();
builder.Services.AddHostedService<ExpiryWorker>();
builder.Services.AddHostedService<AgentReportConsumer>();
builder.Services.AddHostedService<AptIndexRefresher>();
builder.Services.AddHostedService<FleetReconciler>();
builder.Services.AddHostedService<FleetOperationSweeper>();
builder.Services.AddHostedService<InventoryWorker>();

var app = builder.Build();

if (!app.Environment.IsDevelopment())
{
    app.UseExceptionHandler("/Error", createScopeForErrors: true);
}

app.UseStatusCodePagesWithReExecute("/not-found", createScopeForStatusCodePages: true);
app.UseAntiforgery();

app.MapStaticAssets();
app.MapRazorComponents<App>()
    .AddInteractiveServerRenderMode();

app.MapHub<VmHub>("/hubs/vm");
app.MapGet("/healthz", () => Results.Ok("ok"));

app.MapGet("/api/identity/ssh", (IdentityService identity) =>
{
    var id = identity.Current;
    var bytes = System.Text.Encoding.UTF8.GetBytes(id.PrivateKeyPem);
    return Results.File(bytes, contentType: "application/x-pem-file", fileDownloadName: "summon_aegis");
});

app.MapGet("/api/identity/ssh.pub", (IdentityService identity) =>
    Results.Text(identity.Current.PublicKeyOpenSsh, contentType: "text/plain"));

app.Run();
