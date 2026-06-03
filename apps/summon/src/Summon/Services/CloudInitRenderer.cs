namespace Summon.Services;

public sealed class CloudInitRenderer(IConfiguration config, IdentityService identity)
{
    public string Render(string hostname, string username, string _password,
                         string rabbitHost, string rabbitUser, string rabbitPass)
    {
        var agentUrl = config["AGENT_DOWNLOAD_URL"]
            ?? "http://minio.minio.svc.cluster.local:9000/vm-images/summon-agent";

        var publicKey = identity.Current.PublicKeyOpenSsh;

        return $"""
            #cloud-config
            hostname: {hostname}
            users:
              - name: {username}
                sudo: ALL=(ALL) NOPASSWD:ALL
                shell: /bin/bash
                lock_passwd: true
                ssh_authorized_keys:
                  - {publicKey}
            ssh_pwauth: false
            disable_root: true
            package_update: true
            packages:
              - qemu-guest-agent
            write_files:
              - path: /etc/systemd/system/summon-agent.service
                content: |
                  [Unit]
                  Description=Summon Agent
                  After=network-online.target
                  Wants=network-online.target
                  [Service]
                  ExecStart=/usr/local/bin/summon-agent
                  Restart=always
                  RestartSec=5
                  Environment=SUMMON_RABBIT_HOST={rabbitHost}
                  Environment=SUMMON_RABBIT_USER={rabbitUser}
                  Environment=SUMMON_RABBIT_PASS={rabbitPass}
                  Environment=SUMMON_VM_NAME={hostname}
                  [Install]
                  WantedBy=multi-user.target
            runcmd:
              - systemctl enable --now qemu-guest-agent
              - curl -fsSL -o /usr/local/bin/summon-agent {agentUrl}
              - chmod +x /usr/local/bin/summon-agent
              - systemctl enable --now summon-agent
            """;
    }
}
