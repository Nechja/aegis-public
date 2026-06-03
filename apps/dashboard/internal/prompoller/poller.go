// Package prompoller periodically queries Prometheus for node/pod/workload metrics.
package prompoller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/Nechja/Aegis/apps/dashboard/internal/state"
)

type Poller struct {
	promURL string
	store   *state.Store
	client  *http.Client
	stop    chan struct{}
}

func Start(promURL string, store *state.Store, interval time.Duration) *Poller {
	p := &Poller{
		promURL: promURL,
		store:   store,
		client:  &http.Client{Timeout: 10 * time.Second},
		stop:    make(chan struct{}),
	}
	go p.run(interval)
	slog.Info("prometheus poller started", "url", promURL, "interval", interval)
	return p
}

func (p *Poller) Stop() { close(p.stop) }

func (p *Poller) run(interval time.Duration) {
	p.poll()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.poll()
		case <-p.stop:
			return
		}
	}
}

func (p *Poller) poll() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p.pollNodeMetrics(ctx)
	p.pollWorkloads(ctx)
}

func (p *Poller) pollNodeMetrics(ctx context.Context) {
	// Node CPU usage (millicores) — group by instance since node_exporter uses instance label
	cpuResults, err := p.query(ctx,
		`sum by (instance) (rate(node_cpu_seconds_total{mode!="idle"}[2m])) * 1000`)
	if err != nil {
		slog.Warn("prom: node cpu query failed", "error", err)
		return
	}

	// Node memory usage (bytes)
	memResults, err := p.query(ctx,
		`node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes`)
	if err != nil {
		slog.Warn("prom: node memory query failed", "error", err)
		return
	}

	// Disk total (root filesystem)
	diskTotalResults, _ := p.query(ctx,
		`node_filesystem_size_bytes{mountpoint="/"}`)

	// Disk usage (root filesystem)
	diskUsageResults, _ := p.query(ctx,
		`node_filesystem_size_bytes{mountpoint="/"} - node_filesystem_avail_bytes{mountpoint="/"}`)

	// Load average (1 min)
	loadResults, _ := p.query(ctx, `node_load1`)

	// Build lookup maps keyed by instance name
	cpuByInst := make(map[string]int64)
	for _, r := range cpuResults { cpuByInst[r.Metric["instance"]] = int64(r.Value) }

	memByInst := make(map[string]int64)
	for _, r := range memResults { memByInst[r.Metric["instance"]] = int64(r.Value) }

	diskTotalByInst := make(map[string]int64)
	for _, r := range diskTotalResults { diskTotalByInst[r.Metric["instance"]] = int64(r.Value) }

	diskUsageByInst := make(map[string]int64)
	for _, r := range diskUsageResults { diskUsageByInst[r.Metric["instance"]] = int64(r.Value) }

	loadByInst := make(map[string]float64)
	for _, r := range loadResults { loadByInst[r.Metric["instance"]] = r.Value }

	// Match instance to node name
	for _, n := range p.store.Nodes() {
		get := func(m map[string]int64) int64 {
			if v := m[n.Name]; v > 0 { return v }
			return m[n.Name+":9100"]
		}
		getF := func(m map[string]float64) float64 {
			if v := m[n.Name]; v > 0 { return v }
			return m[n.Name+":9100"]
		}

		cpu := get(cpuByInst)
		mem := get(memByInst)
		diskTotal := get(diskTotalByInst)
		diskUsage := get(diskUsageByInst)
		load := getF(loadByInst)

		if cpu > 0 || mem > 0 || diskTotal > 0 {
			p.store.UpdateNodeMetrics(n.Name, cpu, mem, diskTotal, diskUsage, load)
		}
	}
}

func (p *Poller) pollWorkloads(ctx context.Context) {
	// Pod info: namespace, pod, node, created_by_name, created_by_kind
	podResults, err := p.query(ctx,
		`count by (namespace, created_by_name, created_by_kind, node) (kube_pod_info{created_by_name!=""})`)
	if err != nil {
		slog.Warn("prom: workload query failed", "error", err)
		return
	}

	// Pod CPU usage by pod
	cpuResults, err := p.query(ctx,
		`sum by (namespace, pod) (rate(container_cpu_usage_seconds_total{container!=""}[2m])) * 1000`)
	if err != nil {
		slog.Warn("prom: pod cpu query failed", "error", err)
	}

	// Pod memory usage by pod
	memResults, err := p.query(ctx,
		`sum by (namespace, pod) (container_memory_working_set_bytes{container!=""})`)
	if err != nil {
		slog.Warn("prom: pod memory query failed", "error", err)
	}

	// Pod-to-owner mapping for aggregating CPU/mem
	podOwner, err := p.query(ctx,
		`kube_pod_info{created_by_name!=""}`)
	if err != nil {
		slog.Warn("prom: pod owner query failed", "error", err)
	}

	// Build owner lookup: namespace/pod → owner name
	ownerMap := make(map[string]string)
	for _, r := range podOwner {
		key := r.Metric["namespace"] + "/" + r.Metric["pod"]
		owner := r.Metric["created_by_name"]
		// Strip ReplicaSet hash to get Deployment name
		if r.Metric["created_by_kind"] == "ReplicaSet" {
			if idx := lastDash(owner); idx > 0 {
				owner = owner[:idx]
			}
		}
		ownerMap[key] = owner
	}

	// Aggregate CPU/mem per workload+node
	type wlKey struct{ owner, namespace, node string }
	cpuSums := make(map[wlKey]int64)
	memSums := make(map[wlKey]int64)

	for _, r := range cpuResults {
		key := r.Metric["namespace"] + "/" + r.Metric["pod"]
		owner := ownerMap[key]
		if owner == "" {
			continue
		}
		// Find node from podOwner
		for _, po := range podOwner {
			if po.Metric["namespace"] == r.Metric["namespace"] && po.Metric["pod"] == r.Metric["pod"] {
				wk := wlKey{owner, r.Metric["namespace"], po.Metric["node"]}
				cpuSums[wk] += int64(r.Value)
				break
			}
		}
	}

	for _, r := range memResults {
		key := r.Metric["namespace"] + "/" + r.Metric["pod"]
		owner := ownerMap[key]
		if owner == "" {
			continue
		}
		for _, po := range podOwner {
			if po.Metric["namespace"] == r.Metric["namespace"] && po.Metric["pod"] == r.Metric["pod"] {
				wk := wlKey{owner, r.Metric["namespace"], po.Metric["node"]}
				memSums[wk] += int64(r.Value)
				break
			}
		}
	}

	// Build workloads
	var workloads []state.Workload
	for _, r := range podResults {
		name := r.Metric["created_by_name"]
		kind := r.Metric["created_by_kind"]
		// Strip ReplicaSet hash
		if kind == "ReplicaSet" {
			kind = "Deployment"
			if idx := lastDash(name); idx > 0 {
				name = name[:idx]
			}
		}

		wk := wlKey{name, r.Metric["namespace"], r.Metric["node"]}
		workloads = append(workloads, state.Workload{
			Name:      name,
			Kind:      kind,
			Namespace: r.Metric["namespace"],
			Node:      r.Metric["node"],
			PodCount:  int(r.Value),
			CPU:       cpuSums[wk],
			Memory:    memSums[wk],
		})
	}

	sort.Slice(workloads, func(i, j int) bool {
		if workloads[i].Namespace != workloads[j].Namespace {
			return workloads[i].Namespace < workloads[j].Namespace
		}
		return workloads[i].Name < workloads[j].Name
	})

	p.store.SetWorkloads(workloads)
	slog.Debug("workloads refreshed from prom", "count", len(workloads))
}

// query executes a PromQL instant query and returns parsed results.
func (p *Poller) query(ctx context.Context, promql string) ([]promResult, error) {
	u := fmt.Sprintf("%s/api/v1/query?query=%s", p.promURL, url.QueryEscape(promql))
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body promResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Status != "success" {
		return nil, fmt.Errorf("prom query failed: %s", body.Status)
	}
	return body.Data.Result, nil
}

type promResponse struct {
	Status string   `json:"status"`
	Data   promData `json:"data"`
}

type promData struct {
	ResultType string       `json:"resultType"`
	Result     []promResult `json:"result"`
}

type promResult struct {
	Metric map[string]string `json:"metric"`
	Value  float64
}

func (r *promResult) UnmarshalJSON(data []byte) error {
	var raw struct {
		Metric map[string]string `json:"metric"`
		Value  [2]json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Metric = raw.Metric
	var valStr string
	if err := json.Unmarshal(raw.Value[1], &valStr); err != nil {
		return err
	}
	r.Value, _ = strconv.ParseFloat(valStr, 64)
	return nil
}

func lastDash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '-' {
			return i
		}
	}
	return -1
}
