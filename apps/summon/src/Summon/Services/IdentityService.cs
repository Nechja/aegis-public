using System.Security.Cryptography;
using System.Text;
using LiteDB;
using Summon.Models;
using Summon.Services.Data;

namespace Summon.Services;

/// <summary>
/// Owns the Summon-wide SSH identity. On first access, generates an RSA-3072 keypair, encodes
/// the public key in OpenSSH <c>authorized_keys</c> format, and persists both halves in LiteDB.
/// Subsequent calls return the cached identity; rotating requires <see cref="Rotate"/>.
/// </summary>
public sealed class IdentityService
{
    private readonly ILiteCollection<Identity> _col;
    private readonly object _lock = new();
    private Identity? _cached;

    public IdentityService(SummonDb db)
    {
        _col = db.Identities;
        _col.EnsureIndex(x => x.Id, unique: true);
    }

    public Identity Current
    {
        get
        {
            if (_cached is not null) return _cached;
            lock (_lock)
            {
                _cached ??= _col.FindAll().FirstOrDefault() ?? Generate();
                return _cached;
            }
        }
    }

    public Identity Rotate()
    {
        lock (_lock)
        {
            foreach (var row in _col.FindAll().ToList())
                _col.Delete(row.Id);
            _cached = Generate();
            return _cached;
        }
    }

    private Identity Generate()
    {
        using var rsa = RSA.Create(3072);
        var parameters = rsa.ExportParameters(includePrivateParameters: false);

        var id = new Identity
        {
            PublicKeyOpenSsh = EncodeOpenSshPublicKey(parameters, comment: "summon-aegis"),
            PrivateKeyPem = ExportPrivateKeyPem(rsa),
            FingerprintSha256 = ComputeSshFingerprint(parameters)
        };
        _col.Insert(id);
        return id;
    }

    private static string ExportPrivateKeyPem(RSA rsa)
    {
        // PKCS#1 "-----BEGIN RSA PRIVATE KEY-----" — widely accepted by OpenSSH's `ssh -i`.
        return rsa.ExportRSAPrivateKeyPem();
    }

    // --- OpenSSH public-key wire format -------------------------------------------------
    // See: https://datatracker.ietf.org/doc/html/rfc4253#section-6.6  (ssh-rsa blob)
    //      https://datatracker.ietf.org/doc/html/rfc4716  (public-key file wrapping)

    private static string EncodeOpenSshPublicKey(RSAParameters p, string comment)
    {
        using var ms = new MemoryStream();
        WriteString(ms, "ssh-rsa"u8);
        WriteMpint(ms, p.Exponent!);
        WriteMpint(ms, p.Modulus!);
        var b64 = Convert.ToBase64String(ms.ToArray());
        return $"ssh-rsa {b64} {comment}";
    }

    private static string ComputeSshFingerprint(RSAParameters p)
    {
        using var ms = new MemoryStream();
        WriteString(ms, "ssh-rsa"u8);
        WriteMpint(ms, p.Exponent!);
        WriteMpint(ms, p.Modulus!);
        var hash = SHA256.HashData(ms.ToArray());
        return "SHA256:" + Convert.ToBase64String(hash).TrimEnd('=');
    }

    private static void WriteString(Stream s, ReadOnlySpan<byte> bytes)
    {
        WriteUInt32Be(s, (uint)bytes.Length);
        s.Write(bytes);
    }

    private static void WriteMpint(Stream s, byte[] value)
    {
        // SSH mpint: omit leading zero bytes, but if the top bit is set, prepend one zero
        // so the value isn't misread as negative (two's complement).
        int start = 0;
        while (start < value.Length - 1 && value[start] == 0) start++;
        var trimmed = value.AsSpan(start);
        bool needsPad = trimmed.Length > 0 && (trimmed[0] & 0x80) != 0;
        int len = trimmed.Length + (needsPad ? 1 : 0);
        WriteUInt32Be(s, (uint)len);
        if (needsPad) s.WriteByte(0x00);
        s.Write(trimmed);
    }

    private static void WriteUInt32Be(Stream s, uint v)
    {
        Span<byte> buf = stackalloc byte[4];
        buf[0] = (byte)(v >> 24);
        buf[1] = (byte)(v >> 16);
        buf[2] = (byte)(v >> 8);
        buf[3] = (byte)v;
        s.Write(buf);
    }
}
