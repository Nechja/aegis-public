using k8s;

namespace Summon.Services;

public sealed class KubeClientFactory
{
    public IKubernetes Client { get; }
    public string VmNamespace { get; }

    public KubeClientFactory(IConfiguration config)
    {
        VmNamespace = config["KUBERNETES_NAMESPACE"] ?? "vms";

        var kubeConfig = KubernetesClientConfiguration.IsInCluster()
            ? KubernetesClientConfiguration.InClusterConfig()
            : KubernetesClientConfiguration.BuildConfigFromConfigFile();

        Client = new Kubernetes(kubeConfig);
    }
}
