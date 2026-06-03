namespace Summon.Models;

/// <summary>
/// Singleton-ish record holding the Summon-managed SSH identity. One row, generated the first
/// time any code touches <c>IdentityService</c>. The private key never leaves the server
/// except through the authenticated download endpoint.
/// </summary>
public sealed class Identity
{
    public Guid Id { get; set; } = Guid.Parse("00000000-0000-0000-0000-000000000001");
    public string PublicKeyOpenSsh { get; set; } = string.Empty;
    public string PrivateKeyPem { get; set; } = string.Empty;
    public string FingerprintSha256 { get; set; } = string.Empty;
    public DateTime CreatedAt { get; set; } = DateTime.UtcNow;
}
