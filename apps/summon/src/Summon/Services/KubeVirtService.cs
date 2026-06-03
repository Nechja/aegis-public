using System.Text.Json;
using k8s;
using k8s.Models;
using Summon.Models;

namespace Summon.Services;

public sealed class KubeVirtService
{
    private readonly IKubernetes _client;
    private readonly string _namespace;
    private readonly string _minioUrl;
    private readonly string _minioBucket;
    private readonly string _storageClass;
    private readonly ILogger<KubeVirtService> _logger;

    public KubeVirtService(KubeClientFactory kube, IConfiguration config, ILogger<KubeVirtService> logger)
    {
        _logger = logger;
        _client = kube.Client;
        _namespace = kube.VmNamespace;
        _minioUrl = config["MINIO_URL"] ?? "http://minio.minio.svc.cluster.local:9000";
        _minioBucket = config["MINIO_BUCKET"] ?? "vm-images";
        _storageClass = config["STORAGE_CLASS"] ?? "longhorn-chonk";
    }

    /// <summary>Default root disk size per instance type. Floor is raised by <see cref="VmTemplate.MinDiskGb"/>.</summary>
    public static int DefaultDiskGbForInstanceType(string instanceType) => instanceType switch
    {
        "small" => 10,
        "medium" => 30,
        "large" => 80,
        _ => 30
    };

    public static int ResolveDiskGb(string instanceType, int templateMinDiskGb) =>
        Math.Max(DefaultDiskGbForInstanceType(instanceType), templateMinDiskGb);

    public async Task<bool> Create(VmRecord vm, VmTemplate template, string cloudInit)
    {
        try
        {
            if (!template.ProvisioningEnabled)
            {
                _logger.LogWarning(
                    "Skipping create for {Name}: template {Template} is catalogued but provisioning is not enabled yet",
                    vm.Name, template.Name);
                return false;
            }

            await CreateDataVolume(vm, template);
            await CreateVirtualMachine(vm, cloudInit);
            await CreateTailscaleService(vm);
            _logger.LogInformation("Created VM resources for {Name}", vm.Name);
            return true;
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to create VM resources for {Name}", vm.Name);
            return false;
        }
    }

    public async Task<bool> Delete(VmRecord vm)
    {
        var success = true;

        try
        {
            await _client.CoreV1.DeleteNamespacedServiceAsync($"vm-{vm.KubeVmName}", _namespace);
        }
        catch (k8s.Autorest.HttpOperationException ex) when (ex.Response.StatusCode == System.Net.HttpStatusCode.NotFound) { }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to delete service for {Name}", vm.Name);
            success = false;
        }

        try
        {
            await _client.CustomObjects.DeleteNamespacedCustomObjectAsync(
                "kubevirt.io", "v1", _namespace, "virtualmachines", vm.KubeVmName);
        }
        catch (k8s.Autorest.HttpOperationException ex) when (ex.Response.StatusCode == System.Net.HttpStatusCode.NotFound) { }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to delete VM for {Name}", vm.Name);
            success = false;
        }

        try
        {
            await _client.CustomObjects.DeleteNamespacedCustomObjectAsync(
                "cdi.kubevirt.io", "v1beta1", _namespace, "datavolumes", $"{vm.KubeVmName}-disk");
        }
        catch (k8s.Autorest.HttpOperationException ex) when (ex.Response.StatusCode == System.Net.HttpStatusCode.NotFound) { }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to delete DataVolume for {Name}", vm.Name);
            success = false;
        }

        return success;
    }

    public async Task<bool> Stop(VmRecord vm)
    {
        try
        {
            var patch = new V1Patch(
                "{\"spec\":{\"running\":false}}",
                V1Patch.PatchType.MergePatch);

            await _client.CustomObjects.PatchNamespacedCustomObjectAsync(
                patch, "kubevirt.io", "v1", _namespace, "virtualmachines", vm.KubeVmName);

            return true;
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to stop VM {Name}", vm.Name);
            return false;
        }
    }

    public async Task<bool> Start(VmRecord vm)
    {
        try
        {
            var patch = new V1Patch(
                "{\"spec\":{\"running\":true}}",
                V1Patch.PatchType.MergePatch);

            await _client.CustomObjects.PatchNamespacedCustomObjectAsync(
                patch, "kubevirt.io", "v1", _namespace, "virtualmachines", vm.KubeVmName);

            return true;
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to start VM {Name}", vm.Name);
            return false;
        }
    }

    public async Task<string?> GetVmiPhase(string kubeVmName)
    {
        try
        {
            var result = await _client.CustomObjects.GetNamespacedCustomObjectAsync(
                "kubevirt.io", "v1", _namespace, "virtualmachineinstances", kubeVmName);

            using var doc = JsonSerializer.SerializeToDocument(result);
            return doc.RootElement
                .GetProperty("status")
                .GetProperty("phase")
                .GetString();
        }
        catch
        {
            return null;
        }
    }

    private async Task CreateDataVolume(VmRecord vm, VmTemplate template)
    {
        var imageUrl = $"{_minioUrl}/{_minioBucket}/{template.BaseImage}";
        var diskGb = vm.DiskGb > 0 ? vm.DiskGb : ResolveDiskGb(vm.InstanceType, template.MinDiskGb);

        var dv = new Dictionary<string, object>
        {
            ["apiVersion"] = "cdi.kubevirt.io/v1beta1",
            ["kind"] = "DataVolume",
            ["metadata"] = new Dictionary<string, object>
            {
                ["name"] = $"{vm.KubeVmName}-disk",
                ["namespace"] = _namespace
            },
            ["spec"] = new Dictionary<string, object>
            {
                ["source"] = new Dictionary<string, object>
                {
                    ["http"] = new Dictionary<string, object>
                    {
                        ["url"] = imageUrl
                    }
                },
                ["storage"] = new Dictionary<string, object>
                {
                    ["accessModes"] = new[] { "ReadWriteOnce" },
                    ["volumeMode"] = "Filesystem",
                    ["resources"] = new Dictionary<string, object>
                    {
                        ["requests"] = new Dictionary<string, object>
                        {
                            ["storage"] = $"{diskGb}Gi"
                        }
                    },
                    ["storageClassName"] = _storageClass
                }
            }
        };

        try
        {
            await _client.CustomObjects.CreateNamespacedCustomObjectAsync(
                dv, "cdi.kubevirt.io", "v1beta1", _namespace, "datavolumes");
        }
        catch (k8s.Autorest.HttpOperationException ex) when (ex.Response.StatusCode == System.Net.HttpStatusCode.Conflict)
        {
            _logger.LogInformation("DataVolume {Name}-disk already exists; reusing", vm.KubeVmName);
        }
    }

    private async Task CreateVirtualMachine(VmRecord vm, string cloudInit)
    {
        var (cores, memory) = vm.InstanceType switch
        {
            "small" => (1, "1Gi"),
            "medium" => (2, "4Gi"),
            "large" => (4, "8Gi"),
            _ => (2, "4Gi")
        };

        var vmResource = new Dictionary<string, object>
        {
            ["apiVersion"] = "kubevirt.io/v1",
            ["kind"] = "VirtualMachine",
            ["metadata"] = new Dictionary<string, object>
            {
                ["name"] = vm.KubeVmName,
                ["namespace"] = _namespace,
                ["labels"] = new Dictionary<string, object>
                {
                    ["app"] = "summon",
                    ["summon/vm-id"] = vm.Id.ToString(),
                    ["summon.aegis/managed"] = "true"
                },
                ["annotations"] = new Dictionary<string, object>
                {
                    ["summon.aegis/template"] = vm.TemplateName,
                    ["summon.aegis/instance-type"] = vm.InstanceType,
                    ["summon.aegis/disk-gb"] = vm.DiskGb.ToString(),
                    ["summon.aegis/expires-at"] = vm.ExpiresAt?.ToString("o") ?? "none",
                    ["summon.aegis/packages"] = JsonSerializer.Serialize(vm.Packages)
                }
            },
            ["spec"] = new Dictionary<string, object>
            {
                ["running"] = true,
                ["template"] = new Dictionary<string, object>
                {
                    ["metadata"] = new Dictionary<string, object>
                    {
                        ["labels"] = new Dictionary<string, object>
                        {
                            ["kubevirt.io/vm"] = vm.KubeVmName
                        }
                    },
                    ["spec"] = new Dictionary<string, object>
                    {
                        ["nodeSelector"] = new Dictionary<string, object>
                        {
                            ["kubernetes.io/arch"] = vm.Arch
                        },
                        ["domain"] = new Dictionary<string, object>
                        {
                            ["cpu"] = new Dictionary<string, object>
                            {
                                ["cores"] = cores
                            },
                            ["memory"] = new Dictionary<string, object>
                            {
                                ["guest"] = memory
                            },
                            ["devices"] = new Dictionary<string, object>
                            {
                                ["disks"] = new object[]
                                {
                                    new Dictionary<string, object>
                                    {
                                        ["name"] = "rootdisk",
                                        ["disk"] = new Dictionary<string, object> { ["bus"] = "virtio" }
                                    },
                                    new Dictionary<string, object>
                                    {
                                        ["name"] = "cloudinitdisk",
                                        ["disk"] = new Dictionary<string, object> { ["bus"] = "virtio" }
                                    }
                                },
                                ["interfaces"] = new object[]
                                {
                                    new Dictionary<string, object>
                                    {
                                        ["name"] = "default",
                                        ["masquerade"] = new Dictionary<string, object>()
                                    }
                                }
                            },
                            ["machine"] = new Dictionary<string, object>
                            {
                                ["type"] = "q35"
                            }
                        },
                        ["networks"] = new object[]
                        {
                            new Dictionary<string, object>
                            {
                                ["name"] = "default",
                                ["pod"] = new Dictionary<string, object>()
                            }
                        },
                        ["volumes"] = new object[]
                        {
                            new Dictionary<string, object>
                            {
                                ["name"] = "rootdisk",
                                ["dataVolume"] = new Dictionary<string, object>
                                {
                                    ["name"] = $"{vm.KubeVmName}-disk"
                                }
                            },
                            new Dictionary<string, object>
                            {
                                ["name"] = "cloudinitdisk",
                                ["cloudInitNoCloud"] = new Dictionary<string, object>
                                {
                                    ["userData"] = cloudInit
                                }
                            }
                        }
                    }
                }
            }
        };

        try
        {
            await _client.CustomObjects.CreateNamespacedCustomObjectAsync(
                vmResource, "kubevirt.io", "v1", _namespace, "virtualmachines");
        }
        catch (k8s.Autorest.HttpOperationException ex) when (ex.Response.StatusCode == System.Net.HttpStatusCode.Conflict)
        {
            _logger.LogInformation("VirtualMachine {Name} already exists; reusing", vm.KubeVmName);
        }
    }

    private async Task CreateTailscaleService(VmRecord vm)
    {
        var service = new V1Service
        {
            Metadata = new V1ObjectMeta
            {
                Name = $"vm-{vm.KubeVmName}",
                NamespaceProperty = _namespace,
                Annotations = new Dictionary<string, string>
                {
                    ["tailscale.com/expose"] = "true",
                    ["tailscale.com/hostname"] = vm.TailscaleHostname
                }
            },
            Spec = new V1ServiceSpec
            {
                Type = "LoadBalancer",
                LoadBalancerClass = "tailscale",
                Selector = new Dictionary<string, string>
                {
                    ["kubevirt.io/vm"] = vm.KubeVmName
                },
                Ports =
                [
                    new V1ServicePort
                    {
                        Port = 22,
                        TargetPort = 22,
                        Name = "ssh"
                    }
                ]
            }
        };

        try
        {
            await _client.CoreV1.CreateNamespacedServiceAsync(service, _namespace);
        }
        catch (k8s.Autorest.HttpOperationException ex) when (ex.Response.StatusCode == System.Net.HttpStatusCode.Conflict)
        {
            _logger.LogInformation("Service vm-{Name} already exists; reusing", vm.KubeVmName);
        }
    }
}
