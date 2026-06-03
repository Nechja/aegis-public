// Package state defines the unified cluster state that all data sources contribute to.
package state

import (
	"sync"
	"time"
)

type Service struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	URL       string `json:"url"`
	Hostname  string `json:"hostname"`
}

type NodeInfo struct {
	Name          string            `json:"name"`
	Arch          string            `json:"arch"`
	OS            string            `json:"os"`
	Roles         []string          `json:"roles"`
	Ready         bool              `json:"ready"`
	Unschedulable bool              `json:"unschedulable"`
	GPU           string            `json:"gpu"` // "", "true", "igpu"
	WorkloadClass string            `json:"workloadClass"`
	Labels        map[string]string `json:"labels"`
	CPUCores      int64             `json:"cpuCores"`
	MemoryBytes   int64             `json:"memoryBytes"`
	CPUUsage      int64             `json:"cpuUsage"`
	MemoryUsage   int64             `json:"memoryUsage"`
	DiskBytes     int64             `json:"diskBytes"`
	DiskUsage     int64             `json:"diskUsage"`
	LoadAvg       float64           `json:"loadAvg"`
}

type Workload struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Node      string `json:"node"`
	PodCount  int    `json:"podCount"`
	CPU       int64  `json:"cpu"`
	Memory    int64  `json:"memory"`
}

type NetPolicyRule struct {
	Namespace string   `json:"namespace"`  // policy's namespace
	Name      string   `json:"name"`       // policy name
	Direction string   `json:"direction"`  // "ingress" or "egress"
	Peers     []string `json:"peers"`      // namespace names or CIDRs this rule allows
	Ports     []int32  `json:"ports"`      // allowed ports
}

type ClusterEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Object    string    `json:"object"`
	Namespace string    `json:"namespace"`
	Message   string    `json:"message"`
}

type TopPod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	CPU       int64  `json:"cpu"`
	Memory    int64  `json:"memory"`
}

// Store holds the unified cluster state. Multiple goroutines write to it,
// the server reads from it. All access is mutex-protected.
type Store struct {
	mu        sync.RWMutex
	services  []Service
	nodes     []NodeInfo
	workloads []Workload
	events      []ClusterEvent
	netPolicies []NetPolicyRule
	listeners   []func()
	lastUpdate time.Time
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) OnChange(fn func()) {
	s.listeners = append(s.listeners, fn)
}

func (s *Store) notify() {
	s.lastUpdate = time.Now()
	for _, fn := range s.listeners {
		fn()
	}
}

// LastUpdate returns when the store was last written to.
func (s *Store) LastUpdate() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastUpdate
}

// IsHealthy returns true if the store has been updated within the given duration.
func (s *Store) IsHealthy(maxAge time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastUpdate.IsZero() {
		return false
	}
	return time.Since(s.lastUpdate) < maxAge
}

// Writers

// SetCollectorData updates services, nodes (preserving metrics), and events in one batch.
func (s *Store) SetCollectorData(services []Service, nodes []NodeInfo, events []ClusterEvent) {
	s.mu.Lock()
	if len(services) > 0 {
		s.services = services
	}
	if len(nodes) > 0 {
		// Preserve existing CPU/memory metrics when merging node info
		existing := make(map[string]*NodeInfo, len(s.nodes))
		for i := range s.nodes {
			existing[s.nodes[i].Name] = &s.nodes[i]
		}
		for i := range nodes {
			if prev, ok := existing[nodes[i].Name]; ok {
				nodes[i].CPUUsage = prev.CPUUsage
				nodes[i].MemoryUsage = prev.MemoryUsage
				nodes[i].DiskBytes = prev.DiskBytes
				nodes[i].DiskUsage = prev.DiskUsage
				nodes[i].LoadAvg = prev.LoadAvg
			}
		}
		s.nodes = nodes
	}
	if len(events) > 0 {
		s.events = events
	}
	s.mu.Unlock()
	s.notify()
}

func (s *Store) SetServices(v []Service) {
	s.mu.Lock()
	s.services = v
	s.mu.Unlock()
	s.notify()
}

func (s *Store) SetNodes(v []NodeInfo) {
	s.mu.Lock()
	s.nodes = v
	s.mu.Unlock()
	s.notify()
}

func (s *Store) UpdateNodeMetrics(name string, cpuUsage, memUsage, diskTotal, diskUsage int64, loadAvg float64) {
	s.mu.Lock()
	for i := range s.nodes {
		if s.nodes[i].Name == name {
			s.nodes[i].CPUUsage = cpuUsage
			s.nodes[i].MemoryUsage = memUsage
			if diskTotal > 0 {
				s.nodes[i].DiskBytes = diskTotal
				s.nodes[i].DiskUsage = diskUsage
			}
			if loadAvg > 0 {
				s.nodes[i].LoadAvg = loadAvg
			}
			break
		}
	}
	s.mu.Unlock()
	s.notify()
}

func (s *Store) SetWorkloads(v []Workload) {
	s.mu.Lock()
	s.workloads = v
	s.mu.Unlock()
	s.notify()
}

func (s *Store) SetNetPolicies(v []NetPolicyRule) {
	s.mu.Lock()
	s.netPolicies = v
	s.mu.Unlock()
	s.notify()
}

func (s *Store) NetPolicies() []NetPolicyRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NetPolicyRule, len(s.netPolicies))
	copy(out, s.netPolicies)
	return out
}

func (s *Store) SetEvents(v []ClusterEvent) {
	s.mu.Lock()
	s.events = v
	s.mu.Unlock()
	s.notify()
}

// Readers

func (s *Store) Services() []Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Service, len(s.services))
	copy(out, s.services)
	return out
}

func (s *Store) Nodes() []NodeInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NodeInfo, len(s.nodes))
	copy(out, s.nodes)
	return out
}

func (s *Store) Workloads() []Workload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Workload, len(s.workloads))
	copy(out, s.workloads)
	return out
}

func (s *Store) Events() []ClusterEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ClusterEvent, len(s.events))
	copy(out, s.events)
	return out
}
