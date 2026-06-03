// Package informers watches ingresses and nodes via k8s informers.
// These are the only resources that need live reactivity.
package informers

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/Nechja/Aegis/apps/dashboard/internal/state"
)

type Watcher struct {
	store  *state.Store
	domain string
	stop   chan struct{}
}

func Start(client kubernetes.Interface, store *state.Store, domain string) *Watcher {
	w := &Watcher{
		store:  store,
		domain: domain,
		stop:   make(chan struct{}),
	}

	factory := informers.NewSharedInformerFactory(client, 30*time.Second)

	ingressInf := factory.Networking().V1().Ingresses().Informer()
	ingressInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { w.rebuildIngresses(ingressInf.GetStore()) },
		UpdateFunc: func(_, _ interface{}) { w.rebuildIngresses(ingressInf.GetStore()) },
		DeleteFunc: func(_ interface{}) { w.rebuildIngresses(ingressInf.GetStore()) },
	})

	nodeInf := factory.Core().V1().Nodes().Informer()
	nodeInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { w.rebuildNodes(nodeInf.GetStore()) },
		UpdateFunc: func(_, _ interface{}) { w.rebuildNodes(nodeInf.GetStore()) },
		DeleteFunc: func(_ interface{}) { w.rebuildNodes(nodeInf.GetStore()) },
	})

	netpolInf := factory.Networking().V1().NetworkPolicies().Informer()
	netpolInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(_ interface{}) { w.rebuildNetPolicies(netpolInf.GetStore()) },
		UpdateFunc: func(_, _ interface{}) { w.rebuildNetPolicies(netpolInf.GetStore()) },
		DeleteFunc: func(_ interface{}) { w.rebuildNetPolicies(netpolInf.GetStore()) },
	})

	factory.Start(w.stop)

	// Non-blocking — caches sync in background via event handlers
	go func() {
		factory.WaitForCacheSync(w.stop)
		slog.Info("informer caches synced")
	}()

	slog.Info("informers started", "domain", domain)
	return w
}

func (w *Watcher) Stop() {
	close(w.stop)
}

func (w *Watcher) rebuildIngresses(store cache.Store) {
	var svcs []state.Service
	for _, obj := range store.List() {
		ing, ok := obj.(*networkingv1.Ingress)
		if !ok || ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != "tailscale" {
			continue
		}

		hostname := ing.Name
		if len(ing.Spec.TLS) > 0 && len(ing.Spec.TLS[0].Hosts) > 0 {
			hostname = ing.Spec.TLS[0].Hosts[0]
		}

		svcs = append(svcs, state.Service{
			Name:      ing.Name,
			Namespace: ing.Namespace,
			URL:       fmt.Sprintf("https://%s.%s", hostname, w.domain),
			Hostname:  fmt.Sprintf("%s.%s", hostname, w.domain),
		})
	}
	sort.Slice(svcs, func(i, j int) bool { return svcs[i].Name < svcs[j].Name })
	w.store.SetServices(svcs)
	slog.Debug("ingresses refreshed", "count", len(svcs))
}

func (w *Watcher) rebuildNodes(store cache.Store) {
	var nodes []state.NodeInfo
	for _, obj := range store.List() {
		node, ok := obj.(*corev1.Node)
		if !ok {
			continue
		}

		info := state.NodeInfo{
			Name:          node.Name,
			Arch:          node.Labels["kubernetes.io/arch"],
			OS:            node.Labels["kubernetes.io/os"],
			Unschedulable: node.Spec.Unschedulable,
			GPU:           node.Labels["gpu"],
			WorkloadClass: node.Labels["workload-class"],
			Labels: map[string]string{
				"storage":  node.Labels["storage"],
				"gpu":      node.Labels["gpu"],
				"kubevirt": node.Labels["kubevirt.io/schedulable"],
			},
		}

		for label := range node.Labels {
			const prefix = "node-role.kubernetes.io/"
			if strings.HasPrefix(label, prefix) {
				info.Roles = append(info.Roles, strings.TrimPrefix(label, prefix))
			}
		}
		if len(info.Roles) == 0 {
			info.Roles = []string{"worker"}
		}

		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				info.Ready = cond.Status == corev1.ConditionTrue
				break
			}
		}

		if cpu := node.Status.Capacity.Cpu(); cpu != nil {
			info.CPUCores = cpu.MilliValue()
		}
		if mem := node.Status.Capacity.Memory(); mem != nil {
			info.MemoryBytes = mem.Value()
		}

		nodes = append(nodes, info)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	w.store.SetNodes(nodes)
	slog.Debug("nodes refreshed", "count", len(nodes))
}

func (w *Watcher) rebuildNetPolicies(store cache.Store) {
	var rules []state.NetPolicyRule
	for _, obj := range store.List() {
		np, ok := obj.(*networkingv1.NetworkPolicy)
		if !ok {
			continue
		}

		// Parse ingress rules
		for _, ingress := range np.Spec.Ingress {
			var peers []string
			for _, from := range ingress.From {
				if from.NamespaceSelector != nil {
					for k, v := range from.NamespaceSelector.MatchLabels {
						if k == "kubernetes.io/metadata.name" {
							peers = append(peers, v)
						}
					}
					if from.NamespaceSelector.MatchLabels == nil || len(from.NamespaceSelector.MatchLabels) == 0 {
						peers = append(peers, "*")
					}
				}
				if from.IPBlock != nil {
					peers = append(peers, from.IPBlock.CIDR)
				}
			}
			var ports []int32
			for _, p := range ingress.Ports {
				if p.Port != nil {
					ports = append(ports, p.Port.IntVal)
				}
			}
			if len(peers) > 0 || len(ports) > 0 {
				rules = append(rules, state.NetPolicyRule{
					Namespace: np.Namespace,
					Name:      np.Name,
					Direction: "ingress",
					Peers:     peers,
					Ports:     ports,
				})
			}
		}

		// Parse egress rules
		for _, egress := range np.Spec.Egress {
			var peers []string
			for _, to := range egress.To {
				if to.NamespaceSelector != nil {
					for k, v := range to.NamespaceSelector.MatchLabels {
						if k == "kubernetes.io/metadata.name" {
							peers = append(peers, v)
						}
					}
					if to.NamespaceSelector.MatchLabels == nil || len(to.NamespaceSelector.MatchLabels) == 0 {
						peers = append(peers, "*")
					}
				}
				if to.IPBlock != nil {
					peers = append(peers, to.IPBlock.CIDR)
				}
			}
			var ports []int32
			for _, p := range egress.Ports {
				if p.Port != nil {
					ports = append(ports, p.Port.IntVal)
				}
			}
			if len(peers) > 0 || len(ports) > 0 {
				rules = append(rules, state.NetPolicyRule{
					Namespace: np.Namespace,
					Name:      np.Name,
					Direction: "egress",
					Peers:     peers,
					Ports:     ports,
				})
			}
		}
	}

	w.store.SetNetPolicies(rules)
	slog.Debug("network policies refreshed", "count", len(rules))
}
