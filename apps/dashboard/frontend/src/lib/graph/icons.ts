// MDI icon paths for workload types and known services.
// Each returns an SVG path string from @mdi/js.

import {
	mdiKubernetes,
	mdiShieldLock,
	mdiShieldAlert,
	mdiDatabase,
	mdiHarddisk,
	mdiChartLine,
	mdiEye,
	mdiGit,
	mdiCertificate,
	mdiLock,
	mdiRocketLaunch,
	mdiLan,
	mdiSecurity,
	mdiBug,
	mdiScaleBalance,
	mdiCog,
	mdiDns,
	mdiWeb,
	mdiGauge,
	mdiConnection,
	mdiPackageVariantClosed,
} from '@mdi/js';

import type { GraphNode } from './types';

// Match by workload name → specific icon
const NAME_ICONS: Record<string, string> = {
	'grafana': mdiGauge,
	'prometheus': mdiChartLine,
	'prometheus-k8s': mdiChartLine,
	'prometheus-operator': mdiChartLine,
	'prometheus-adapter': mdiChartLine,
	'alertmanager-main': mdiShieldAlert,
	'loki': mdiDatabase,
	'argocd-server': mdiGit,
	'argocd-repo-server': mdiGit,
	'argocd-application-controller': mdiGit,
	'argocd-applicationset-controller': mdiGit,
	'argocd-dex-server': mdiLock,
	'argocd-redis': mdiDatabase,
	'argocd-notifications-controller': mdiGit,
	'cert-manager': mdiCertificate,
	'cert-manager-webhook': mdiCertificate,
	'cert-manager-cainjector': mdiCertificate,
	'sealed-secrets-controller': mdiLock,
	'linkerd-destination': mdiConnection,
	'linkerd-identity': mdiLock,
	'linkerd-proxy-injector': mdiConnection,
	'metrics-api': mdiChartLine,
	'tap': mdiEye,
	'tap-injector': mdiEye,
	'web': mdiWeb,
	'crowdsec-lapi': mdiShieldLock,
	'crowdsec-agent': mdiShieldLock,
	'falco': mdiBug,
	'trivy-operator': mdiSecurity,
	'kyverno-admission-controller': mdiScaleBalance,
	'kyverno-background-controller': mdiScaleBalance,
	'kyverno-cleanup-controller': mdiScaleBalance,
	'kyverno-reports-controller': mdiScaleBalance,
	'longhorn-manager': mdiHarddisk,
	'longhorn-driver-deployer': mdiHarddisk,
	'longhorn-ui': mdiHarddisk,
	'longhorn-csi-plugin': mdiHarddisk,
	'coredns': mdiDns,
	'metrics-server': mdiChartLine,
	'local-path-provisioner': mdiHarddisk,
	'kube-state-metrics': mdiChartLine,
	'node-exporter': mdiChartLine,
	'blackbox-exporter': mdiChartLine,
	'alloy': mdiChartLine,
	'operator': mdiLan,
	'keda-operator': mdiRocketLaunch,
	'keda-admission': mdiRocketLaunch,
	'keda-metrics-apiserver': mdiRocketLaunch,
	'aegis-dashboard': mdiGauge,
};

// Match by namespace → fallback icon
const NS_ICONS: Record<string, string> = {
	'monitoring': mdiChartLine,
	'argocd': mdiGit,
	'linkerd': mdiConnection,
	'linkerd-viz': mdiEye,
	'longhorn-system': mdiHarddisk,
	'crowdsec': mdiShieldLock,
	'falco': mdiBug,
	'kyverno': mdiScaleBalance,
	'cert-manager': mdiCertificate,
	'sealed-secrets': mdiLock,
	'tailscale': mdiLan,
	'trivy-system': mdiSecurity,
	'keda': mdiRocketLaunch,
};

// Match by workload kind → fallback
const KIND_ICONS: Record<string, string> = {
	'DaemonSet': mdiCog,
	'StatefulSet': mdiDatabase,
	'Job': mdiRocketLaunch,
};

export function iconPathFor(n: GraphNode): string {
	if (n.kind === 'node') return mdiKubernetes;
	const byName = NAME_ICONS[n.label];
	if (byName) return byName;
	const byNs = NS_ICONS[n.meta.namespace];
	if (byNs) return byNs;
	const byKind = KIND_ICONS[n.meta.kind];
	if (byKind) return byKind;
	return mdiPackageVariantClosed;
}
