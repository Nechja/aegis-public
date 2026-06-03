package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/Nechja/Aegis/apps/dashboard/internal/state"
)

func (s *Server) handleMermaid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	ns := r.URL.Query().Get("namespace")
	if ns != "" {
		w.Write([]byte(s.buildNamespaceDiagram(ns)))
	} else {
		w.Write([]byte(s.buildOverview()))
	}
}

// buildOverview generates a high-level cluster diagram — namespaces as groups, nodes, Tailscale services.
func (s *Server) buildOverview() string {
	nodes := s.store.Nodes()
	workloads := s.store.Workloads()
	services := s.store.Services()

	serviceNs := make(map[string][]state.Service)
	for _, svc := range services {
		serviceNs[svc.Namespace] = append(serviceNs[svc.Namespace], svc)
	}

	// Count workloads per namespace
	nsCounts := make(map[string]int)
	seen := make(map[string]bool)
	for _, wl := range workloads {
		key := wl.Namespace + "/" + wl.Name
		if !seen[key] {
			seen[key] = true
			nsCounts[wl.Namespace]++
		}
	}

	nsList := make([]string, 0, len(nsCounts))
	for ns := range nsCounts {
		nsList = append(nsList, ns)
	}
	sort.Strings(nsList)

	var b strings.Builder
	b.WriteString("graph TB\n\n")
	writeStyles(&b)

	// Tailscale ingress
	b.WriteString("  subgraph ingress[\"Tailscale Ingress\"]\n")
	b.WriteString("    direction LR\n")
	for _, svc := range services {
		b.WriteString(fmt.Sprintf("    ts_%s>\"🔗 %s\"]:::svc_ext\n", san(svc.Name), svc.Name))
	}
	b.WriteString("  end\n\n")

	// Nodes
	b.WriteString("  subgraph hw[\"Nodes\"]\n")
	b.WriteString("    direction LR\n")
	for _, node := range nodes {
		b.WriteString(fmt.Sprintf("    %s{{\"🖥 %s\\n%s\"}}:::node_hw\n",
			san(node.Name), node.Name, node.Arch))
	}
	b.WriteString("  end\n\n")

	// Namespaces as summary boxes
	b.WriteString("  subgraph platform[\"Platform\"]\n")
	b.WriteString("    direction LR\n")
	for _, ns := range nsList {
		nsID := san(ns)
		count := nsCounts[ns]
		b.WriteString(fmt.Sprintf("    %s[\"%s\\n%d workloads\"]:::%s\n",
			nsID, ns, count, nsClass(ns)))
	}
	b.WriteString("  end\n\n")

	// Service → namespace links
	for _, svc := range services {
		b.WriteString(fmt.Sprintf("  ts_%s --> %s\n", san(svc.Name), san(svc.Namespace)))
	}

	return b.String()
}

// buildNamespaceDiagram generates a detailed diagram for a single namespace.
func (s *Server) buildNamespaceDiagram(ns string) string {
	workloads := s.store.Workloads()
	services := s.store.Services()
	nodes := s.store.Nodes()

	serviceSet := make(map[string]state.Service)
	for _, svc := range services {
		serviceSet[svc.Name] = svc
	}

	// Collect workloads for this namespace, deduplicated, with node info
	type wlInfo struct {
		name  string
		kind  string
		nodes []string
	}
	wlMap := make(map[string]*wlInfo)
	for _, wl := range workloads {
		if wl.Namespace != ns {
			continue
		}
		existing := wlMap[wl.Name]
		if existing == nil {
			existing = &wlInfo{name: wl.Name, kind: wl.Kind}
			wlMap[wl.Name] = existing
		}
		found := false
		for _, n := range existing.nodes {
			if n == wl.Node {
				found = true
				break
			}
		}
		if !found {
			existing.nodes = append(existing.nodes, wl.Node)
		}
	}

	wlList := make([]*wlInfo, 0, len(wlMap))
	for _, wl := range wlMap {
		wlList = append(wlList, wl)
	}
	sort.Slice(wlList, func(i, j int) bool { return wlList[i].name < wlList[j].name })

	var b strings.Builder
	b.WriteString("graph TB\n\n")
	writeStyles(&b)

	// Namespace subgraph with all workloads
	nsID := san(ns)
	cls := nsClass(ns)
	b.WriteString(fmt.Sprintf("  subgraph %s[\"%s\"]\n", nsID, ns))

	for _, wl := range wlList {
		wlID := fmt.Sprintf("%s_%s", nsID, san(wl.name))
		label := wl.name
		if wl.kind != "Deployment" {
			label = fmt.Sprintf("%s\\n(%s)", wl.name, wl.kind)
		}
		if _, isSvc := serviceSet[wl.name]; isSvc {
			b.WriteString(fmt.Sprintf("    %s([\"%s\"]):::%s\n", wlID, label, cls))
		} else {
			b.WriteString(fmt.Sprintf("    %s[\"%s\"]:::%s\n", wlID, label, cls))
		}
	}
	b.WriteString("  end\n\n")

	// Nodes that run workloads from this namespace
	activeNodes := make(map[string]bool)
	for _, wl := range wlList {
		for _, n := range wl.nodes {
			activeNodes[n] = true
		}
	}
	b.WriteString("  subgraph hw[\"Nodes\"]\n")
	b.WriteString("    direction LR\n")
	for _, node := range nodes {
		if !activeNodes[node.Name] {
			continue
		}
		b.WriteString(fmt.Sprintf("    %s{{\"🖥 %s\"}}:::node_hw\n", san(node.Name), node.Name))
	}
	b.WriteString("  end\n\n")

	// Workload → node placement links
	for _, wl := range wlList {
		wlID := fmt.Sprintf("%s_%s", nsID, san(wl.name))
		for _, nodeName := range wl.nodes {
			b.WriteString(fmt.Sprintf("  %s -.-> %s\n", wlID, san(nodeName)))
		}
	}

	// Tailscale service links
	b.WriteString("\n")
	for _, wl := range wlList {
		if svc, ok := serviceSet[wl.name]; ok {
			tsID := fmt.Sprintf("ts_%s", san(svc.Name))
			wlID := fmt.Sprintf("%s_%s", nsID, san(wl.name))
			b.WriteString(fmt.Sprintf("  %s>\"🔗 %s\"]:::svc_ext --> %s\n", tsID, svc.Hostname, wlID))
		}
	}

	return b.String()
}

func writeStyles(b *strings.Builder) {
	b.WriteString("  classDef node_hw fill:#161b22,stroke:#00AFAF,color:#e6edf3,stroke-width:2px\n")
	b.WriteString("  classDef svc_ext fill:#0a0a0f,stroke:#00AFAF,color:#00AFAF,stroke-dasharray:5 5\n")
	b.WriteString("  classDef argocd fill:#1a1025,stroke:#D787D7,color:#D787D7\n")
	b.WriteString("  classDef monitoring fill:#1a2510,stroke:#5FAF00,color:#5FAF00\n")
	b.WriteString("  classDef workloads fill:#102525,stroke:#00AFAF,color:#00AFAF\n")
	b.WriteString("  classDef linkerd fill:#251a10,stroke:#FF8700,color:#FF8700\n")
	b.WriteString("  classDef linkerd_viz fill:#251a10,stroke:#FF8700,color:#FF8700\n")
	b.WriteString("  classDef longhorn_system fill:#252010,stroke:#FFAF5F,color:#FFAF5F\n")
	b.WriteString("  classDef kube_system fill:#1a1a1a,stroke:#6C6C6C,color:#8b949e\n")
	b.WriteString("  classDef falco fill:#250a15,stroke:#AF005F,color:#AF005F\n")
	b.WriteString("  classDef crowdsec fill:#250a15,stroke:#AF005F,color:#AF005F\n")
	b.WriteString("  classDef kyverno fill:#1a1525,stroke:#AF5F87,color:#AF5F87\n")
	b.WriteString("  classDef cert_manager fill:#1a1a1a,stroke:#6C6C6C,color:#8b949e\n")
	b.WriteString("  classDef sealed_secrets fill:#1a1a1a,stroke:#6C6C6C,color:#8b949e\n")
	b.WriteString("  classDef tailscale fill:#102525,stroke:#00AFAF,color:#00AFAF\n")
	b.WriteString("  classDef keda fill:#252010,stroke:#FFAF5F,color:#FFAF5F\n")
	b.WriteString("  classDef trivy_system fill:#250a15,stroke:#AF005F,color:#AF005F\n")
	b.WriteString("  classDef redis fill:#251520,stroke:#AF5F87,color:#AF5F87\n")
	b.WriteString("  classDef postgres fill:#102030,stroke:#58a6ff,color:#58a6ff\n")
	b.WriteString("  classDef cnpg_system fill:#102030,stroke:#58a6ff,color:#58a6ff\n")
	b.WriteString("  classDef rabbitmq fill:#252010,stroke:#FFAF5F,color:#FFAF5F\n")
	b.WriteString("  classDef minio fill:#252010,stroke:#FFAF5F,color:#FFAF5F\n")
	b.WriteString("  classDef actions_runner_system fill:#252010,stroke:#FFAF5F,color:#FFAF5F\n")
	b.WriteString("  classDef default_ns fill:#161b22,stroke:#6C6C6C,color:#8b949e\n")
	b.WriteString("\n")
}

func nsClass(ns string) string {
	s := san(ns)
	// known classes
	known := []string{"argocd", "monitoring", "workloads", "linkerd", "linkerd_viz",
		"longhorn_system", "kube_system", "falco", "crowdsec", "kyverno",
		"cert_manager", "sealed_secrets", "tailscale", "keda", "trivy_system",
		"redis", "postgres", "cnpg_system", "rabbitmq", "minio", "actions_runner_system"}
	for _, k := range known {
		if s == k {
			return k
		}
	}
	return "default_ns"
}

func san(s string) string {
	r := strings.NewReplacer("-", "_", ".", "_", "/", "_", " ", "_")
	return r.Replace(s)
}
