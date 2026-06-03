package server

import (
	"embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/Nechja/Aegis/apps/dashboard/internal/state"
)

//go:embed all:frontend/build
var frontend embed.FS

type Server struct {
	mux   *http.ServeMux
	store *state.Store
	hub   *wsHub
}

type DashboardState struct {
	Services  []state.Service      `json:"services"`
	Nodes     []state.NodeInfo     `json:"nodes"`
	Cluster   ClusterSummary       `json:"cluster"`
	Events    []state.ClusterEvent `json:"events"`
	Workloads []state.Workload     `json:"workloads"`
}

type ClusterSummary struct {
	TotalNodes  int   `json:"totalNodes"`
	ReadyNodes  int   `json:"readyNodes"`
	TotalPods   int   `json:"totalPods"`
	TotalCPU    int64 `json:"totalCPU"`
	TotalMemory int64 `json:"totalMemory"`
	UsedCPU     int64 `json:"usedCPU"`
	UsedMemory  int64 `json:"usedMemory"`
	Services    int   `json:"services"`
}

func New(store *state.Store) *Server {
	s := &Server{
		mux:   http.NewServeMux(),
		store: store,
	}

	s.hub = newWSHub(s.stateJSON)
	store.OnChange(s.hub.signalDebounced(100 * time.Millisecond))

	sub, err := fs.Sub(frontend, "frontend/build")
	if err != nil {
		slog.Error("failed to access embedded frontend", "error", err)
	}

	fileServer := http.FileServer(http.FS(sub))
	spaHandler := &spaFileHandler{fs: sub, fileServer: fileServer}

	s.mux.HandleFunc("/api/services", handleJSONFunc(s.store.Services))
	s.mux.HandleFunc("/api/nodes", handleJSONFunc(s.store.Nodes))
	s.mux.HandleFunc("/api/events", handleJSONFunc(s.store.Events))
	s.mux.HandleFunc("/api/cluster", s.handleCluster)
	s.mux.HandleFunc("/api/mermaid", s.handleMermaid)
	s.mux.HandleFunc("/ws", s.hub.handleWS)
	s.mux.HandleFunc("/feed.xml", s.handleRSS)
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	s.mux.Handle("/", spaHandler)

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func handleJSONFunc[T any](fn func() []T) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fn())
	}
}

func (s *Server) handleCluster(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.buildSummary())
}

func (s *Server) buildSummary() ClusterSummary {
	nodes := s.store.Nodes()
	wls := s.store.Workloads()
	totalPods := 0
	for _, wl := range wls {
		totalPods += wl.PodCount
	}
	summary := ClusterSummary{
		TotalNodes: len(nodes),
		TotalPods:  totalPods,
		Services:   len(s.store.Services()),
	}
	for _, n := range nodes {
		if n.Ready {
			summary.ReadyNodes++
		}
		summary.TotalCPU += n.CPUCores
		summary.TotalMemory += n.MemoryBytes
		summary.UsedCPU += n.CPUUsage
		summary.UsedMemory += n.MemoryUsage
	}
	return summary
}

func (s *Server) stateJSON() []byte {
	nodes := s.store.Nodes()
	wls := s.store.Workloads()

	// Attach top 4 pods by CPU to each node
	type topPod struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		CPU       int64  `json:"cpu"`
		Memory    int64  `json:"memory"`
	}
	type nodeWithTop struct {
		state.NodeInfo
		TopPods []topPod `json:"topPods"`
	}

	byNode := make(map[string][]state.Workload)
	for _, wl := range wls {
		byNode[wl.Node] = append(byNode[wl.Node], wl)
	}

	enriched := make([]nodeWithTop, len(nodes))
	for i, n := range nodes {
		enriched[i].NodeInfo = n
		nodeWls := byNode[n.Name]
		sort.Slice(nodeWls, func(a, b int) bool { return nodeWls[a].CPU > nodeWls[b].CPU })
		top := 4
		if len(nodeWls) < top {
			top = len(nodeWls)
		}
		for j := 0; j < top; j++ {
			enriched[i].TopPods = append(enriched[i].TopPods, topPod{
				Name:      nodeWls[j].Name,
				Namespace: nodeWls[j].Namespace,
				CPU:       nodeWls[j].CPU,
				Memory:    nodeWls[j].Memory,
			})
		}
	}

	ds := struct {
		Services    []state.Service       `json:"services"`
		Nodes       []nodeWithTop         `json:"nodes"`
		Cluster     ClusterSummary        `json:"cluster"`
		Events      []state.ClusterEvent  `json:"events"`
		Workloads   []state.Workload      `json:"workloads"`
		NetPolicies []state.NetPolicyRule `json:"netPolicies"`
	}{
		Services:    s.store.Services(),
		Nodes:       enriched,
		Cluster:     s.buildSummary(),
		Events:      s.store.Events(),
		Workloads:   s.store.Workloads(),
		NetPolicies: s.store.NetPolicies(),
	}

	data, _ := json.Marshal(ds)
	return data
}

func (s *Server) handleRSS(w http.ResponseWriter, r *http.Request) {
	events := s.store.Events()
	type rssItem struct {
		XMLName     xml.Name `xml:"item"`
		Title       string   `xml:"title"`
		Description string   `xml:"description"`
		PubDate     string   `xml:"pubDate"`
		GUID        string   `xml:"guid"`
	}
	type rssChannel struct {
		XMLName xml.Name  `xml:"channel"`
		Title   string    `xml:"title"`
		Link    string    `xml:"link"`
		Items   []rssItem `xml:"item"`
	}
	type rssFeed struct {
		XMLName xml.Name   `xml:"rss"`
		Version string     `xml:"version,attr"`
		Channel rssChannel `xml:"channel"`
	}
	items := make([]rssItem, 0, len(events))
	for _, ev := range events {
		items = append(items, rssItem{
			Title:       fmt.Sprintf("[%s] %s — %s", ev.Reason, ev.Object, ev.Namespace),
			Description: ev.Message,
			PubDate:     ev.Timestamp.Format(time.RFC1123Z),
			GUID:        fmt.Sprintf("%s-%s-%s-%d", ev.Namespace, ev.Object, ev.Reason, ev.Timestamp.UnixNano()),
		})
	}
	feed := rssFeed{
		Version: "2.0",
		Channel: rssChannel{
			Title: "Aegis Cluster Issues",
			Link:  "https://" + r.Host,
			Items: items,
		},
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(feed)
}

type spaFileHandler struct {
	fs         fs.FS
	fileServer http.Handler
}

func (h *spaFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	if f, err := h.fs.Open(path[1:]); err == nil {
		f.Close()
		h.fileServer.ServeHTTP(w, r)
		return
	}
	r.URL.Path = "/"
	h.fileServer.ServeHTTP(w, r)
}
