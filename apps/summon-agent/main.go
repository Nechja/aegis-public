package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Config struct {
	RabbitURL string
	VMName    string
}

type InstallItem struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Script string `json:"script"`
}

type Command struct {
	Type        string        `json:"type"`               // "install", "uninstall", "upgrade", "apps.list", "exec", "health", "inventory", "inventory.refresh", "facts"
	OperationId string        `json:"operationId,omitempty"`
	Packages    []string      `json:"packages,omitempty"` // legacy: each entry is the full bash script
	Items       []InstallItem `json:"items,omitempty"`    // preferred: stable id + label + script (install / uninstall)
	Script      string        `json:"script,omitempty"`
}

type App struct {
	Source  string `json:"source"` // "apt" | "snap"
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type Facts struct {
	Kernel       string  `json:"kernel,omitempty"`
	OSPretty     string  `json:"osPretty,omitempty"`
	UptimeSec    int64   `json:"uptimeSec,omitempty"`
	CPUPercent   float64 `json:"cpuPercent,omitempty"`
	CPUCount     int     `json:"cpuCount,omitempty"`
	LoadAvg1     float64 `json:"loadAvg1,omitempty"`
	MemTotalKB   int64   `json:"memTotalKB,omitempty"`
	MemUsedKB    int64   `json:"memUsedKB,omitempty"`
	MemPercent   float64 `json:"memPercent,omitempty"`
	DiskTotalKB  int64   `json:"diskTotalKB,omitempty"`
	DiskUsedKB   int64   `json:"diskUsedKB,omitempty"`
	DiskPercent  float64 `json:"diskPercent,omitempty"`
	PrimaryIP    string  `json:"primaryIp,omitempty"`
	AgentVersion string  `json:"agentVersion,omitempty"`
}

type Pending struct {
	AptUpgradable   int  `json:"aptUpgradable"`
	AptSecurity     int  `json:"aptSecurity"`
	SnapRefreshable int  `json:"snapRefreshable"`
	IndexRefreshed  bool `json:"indexRefreshed,omitempty"` // true when inventory.refresh ran apt-get update
}

type Report struct {
	VMName       string   `json:"vmName"`
	Type         string   `json:"type"` // "heartbeat", "install.progress", "install.done", "uninstall.done", "upgrade.progress", "upgrade.done", "apps.list.done", "exec.result", "agent.ready", "inventory.done", "facts.done"
	OperationId  string   `json:"operationId,omitempty"`
	PackageID    string   `json:"packageId,omitempty"`
	Label        string   `json:"label,omitempty"`
	Package      string   `json:"package,omitempty"` // script snippet (legacy / detail)
	Status       string   `json:"status"`
	Output       string   `json:"output,omitempty"`
	ExitCode     int      `json:"exitCode,omitempty"`
	Timestamp    string   `json:"timestamp"`
	AgentVersion string   `json:"agentVersion,omitempty"`
	Facts        *Facts   `json:"facts,omitempty"`
	Apps         []App    `json:"apps,omitempty"`
	Pending      *Pending `json:"pending,omitempty"`
}

const agentVersion = "0.4.0"
const installTimeout = 45 * time.Minute
const upgradeTimeout = 45 * time.Minute
const inventoryTimeout = 3 * time.Minute
const appsListTimeout = 2 * time.Minute

// aptMu serializes apt/dpkg-touching commands (install / uninstall / upgrade) on this VM
// so parallel commands from the control plane can't deadlock on the dpkg frontend lock.
var aptMu sync.Mutex

func main() {
	cfg := loadConfig()
	log.Printf("summon-agent starting for VM %s", cfg.VMName)

	conn, ch := connectRabbit(cfg.RabbitURL)
	defer conn.Close()
	defer ch.Close()

	commandQueue := fmt.Sprintf("summon.agent.%s", cfg.VMName)
	reportQueue := fmt.Sprintf("summon.report.%s", cfg.VMName)

	declareQueue(ch, commandQueue)
	declareQueue(ch, reportQueue)

	msgs, err := ch.Consume(commandQueue, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("failed to consume: %v", err)
	}

	go heartbeat(ch, reportQueue, cfg.VMName)

	publish(ch, reportQueue, Report{
		VMName:       cfg.VMName,
		Type:         "agent.ready",
		Status:       "ok",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		AgentVersion: agentVersion,
	})

	log.Printf("listening on %s", commandQueue)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sig:
			log.Println("shutting down")
			return
		case msg, ok := <-msgs:
			if !ok {
				log.Println("channel closed, exiting")
				return
			}
			handleCommand(ch, reportQueue, cfg.VMName, msg)
		}
	}
}

func handleCommand(ch *amqp.Channel, reportQueue, vmName string, msg amqp.Delivery) {
	var cmd Command
	if err := json.Unmarshal(msg.Body, &cmd); err != nil {
		log.Printf("invalid command: %v", err)
		msg.Nack(false, false)
		return
	}

	log.Printf("received command: %s", cmd.Type)

	switch cmd.Type {
	case "install":
		if len(cmd.Items) > 0 {
			for _, it := range cmd.Items {
				if strings.TrimSpace(it.Script) == "" {
					continue
				}
				id := strings.TrimSpace(it.ID)
				if id == "" {
					id = "script"
				}
				label := strings.TrimSpace(it.Label)
				if label == "" {
					label = id
				}
				installPackage(ch, reportQueue, vmName, cmd.OperationId, id, label, it.Script)
			}
		} else {
			for _, pkg := range cmd.Packages {
				if strings.TrimSpace(pkg) == "" {
					continue
				}
				installPackage(ch, reportQueue, vmName, cmd.OperationId, "", "", pkg)
			}
		}
	case "uninstall":
		for _, it := range cmd.Items {
			if strings.TrimSpace(it.Script) == "" {
				continue
			}
			id := strings.TrimSpace(it.ID)
			if id == "" {
				id = "script"
			}
			label := strings.TrimSpace(it.Label)
			if label == "" {
				label = id
			}
			uninstallPackage(ch, reportQueue, vmName, cmd.OperationId, id, label, it.Script)
		}
	case "upgrade":
		runUpgrade(ch, reportQueue, vmName, cmd.OperationId)
	case "apps.list":
		runAppsList(ch, reportQueue, vmName, cmd.OperationId)
	case "exec":
		execScript(ch, reportQueue, vmName, cmd.OperationId, cmd.Script)
	case "inventory":
		runInventory(ch, reportQueue, vmName, false)
	case "inventory.refresh":
		runInventory(ch, reportQueue, vmName, true)
	case "facts":
		runFacts(ch, reportQueue, vmName)
	case "health":
		publish(ch, reportQueue, Report{
			VMName:    vmName,
			Type:      "heartbeat",
			Status:    "ok",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}

	msg.Ack(false)
}

func installPackage(ch *amqp.Channel, reportQueue, vmName, operationID, packageID, label, script string) {
	publish(ch, reportQueue, Report{
		VMName:      vmName,
		Type:        "install.progress",
		OperationId: operationID,
		PackageID:   packageID,
		Label:       label,
		Package:     script,
		Status:      "installing",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})

	aptMu.Lock()
	defer aptMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	output, err := cmd.CombinedOutput()

	status := "installed"
	exitCode := 0
	if err != nil {
		status = "failed"
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	publish(ch, reportQueue, Report{
		VMName:      vmName,
		Type:        "install.done",
		OperationId: operationID,
		PackageID:   packageID,
		Label:       label,
		Package:     script,
		Status:      status,
		Output:      truncate(string(output), 4096),
		ExitCode:    exitCode,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})

	logKey := packageID
	if logKey == "" {
		logKey = truncate(script, 80)
	}
	log.Printf("install %s (%s): %s (exit %d)", logKey, label, status, exitCode)
}

func uninstallPackage(ch *amqp.Channel, reportQueue, vmName, operationID, packageID, label, script string) {
	aptMu.Lock()
	defer aptMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	output, err := cmd.CombinedOutput()

	status := "removed"
	exitCode := 0
	if err != nil {
		status = "failed"
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	publish(ch, reportQueue, Report{
		VMName:      vmName,
		Type:        "uninstall.done",
		OperationId: operationID,
		PackageID:   packageID,
		Label:       label,
		Package:     script,
		Status:      status,
		Output:      truncate(string(output), 4096),
		ExitCode:    exitCode,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})

	logKey := packageID
	if logKey == "" {
		logKey = truncate(script, 80)
	}
	log.Printf("uninstall %s (%s): %s (exit %d)", logKey, label, status, exitCode)
}

func runUpgrade(ch *amqp.Channel, reportQueue, vmName, operationID string) {
	publish(ch, reportQueue, Report{
		VMName:      vmName,
		Type:        "upgrade.progress",
		OperationId: operationID,
		Status:      "upgrading",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})

	aptMu.Lock()
	defer aptMu.Unlock()

	script := `set -e
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get upgrade -y \
  -o Dpkg::Options::="--force-confdef" \
  -o Dpkg::Options::="--force-confold"
apt-get autoremove -y || true`

	ctx, cancel := context.WithTimeout(context.Background(), upgradeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	output, err := cmd.CombinedOutput()

	status := "ok"
	exitCode := 0
	if err != nil {
		status = "failed"
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	publish(ch, reportQueue, Report{
		VMName:      vmName,
		Type:        "upgrade.done",
		OperationId: operationID,
		Status:      status,
		Output:      truncate(string(output), 4096),
		ExitCode:    exitCode,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})
	log.Printf("upgrade: %s (exit %d)", status, exitCode)
}

func runAppsList(ch *amqp.Channel, reportQueue, vmName, operationID string) {
	apps := collectApps()

	publish(ch, reportQueue, Report{
		VMName:      vmName,
		Type:        "apps.list.done",
		OperationId: operationID,
		Status:      "ok",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Apps:        apps,
	})
	log.Printf("apps.list: %d packages", len(apps))
}

func collectApps() []App {
	apps := make([]App, 0, 256)

	if _, err := exec.LookPath("dpkg-query"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), appsListTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, "dpkg-query", "-W",
			"-f=${Package}\t${Version}\t${db:Status-Abbrev}\n").Output()
		if err == nil {
			s := bufio.NewScanner(strings.NewReader(string(out)))
			s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for s.Scan() {
				parts := strings.Split(s.Text(), "\t")
				if len(parts) < 3 {
					continue
				}
				status := strings.TrimSpace(parts[2])
				if !strings.HasPrefix(status, "ii") {
					continue
				}
				apps = append(apps, App{Source: "apt", Name: parts[0], Version: parts[1]})
			}
		} else {
			log.Printf("dpkg-query failed: %v", err)
		}
	}

	if _, err := exec.LookPath("snap"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), appsListTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, "snap", "list", "--unicode=never").Output()
		if err == nil {
			s := bufio.NewScanner(strings.NewReader(string(out)))
			first := true
			for s.Scan() {
				if first {
					first = false
					continue
				}
				fields := strings.Fields(s.Text())
				if len(fields) < 2 {
					continue
				}
				apps = append(apps, App{Source: "snap", Name: fields[0], Version: fields[1]})
			}
		}
	}

	return apps
}

func runInventory(ch *amqp.Channel, reportQueue, vmName string, refresh bool) {
	// inventory.refresh runs `apt-get update` first so the upgradable list reflects the
	// current archives. Plain `inventory` reads cached state — cheap, but may be stale.
	if refresh {
		aptMu.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		_ = exec.CommandContext(ctx, "bash", "-c",
			"DEBIAN_FRONTEND=noninteractive apt-get update -qq").Run()
		cancel()
		aptMu.Unlock()
	}

	const script = `set +e
echo "=== apt list --upgradable (first 80 lines) ==="
if command -v apt >/dev/null 2>&1; then
  apt list --upgradable 2>/dev/null | head -n 80
else
  echo "apt not available"
fi
echo ""
echo "=== dpkg (installed .deb count) ==="
if command -v dpkg-query >/dev/null 2>&1; then
  echo -n "ii lines: "
  dpkg-query -l 2>/dev/null | grep -c '^ii' || true
fi
echo ""
echo "=== snap list ==="
if command -v snap >/dev/null 2>&1; then
  snap list 2>/dev/null
else
  echo "snap not installed"
fi
`
	ctx, cancel := context.WithTimeout(context.Background(), inventoryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	output, err := cmd.CombinedOutput()

	status := "ok"
	exitCode := 0
	if err != nil {
		status = "failed"
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	pending := collectPending()
	pending.IndexRefreshed = refresh

	publish(ch, reportQueue, Report{
		VMName:    vmName,
		Type:      "inventory.done",
		Status:    status,
		Output:    truncate(string(output), 16000),
		ExitCode:  exitCode,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Pending:   &pending,
	})
	log.Printf("inventory probe (refresh=%v): %s (exit %d) apt=%d sec=%d snap=%d",
		refresh, status, exitCode, pending.AptUpgradable, pending.AptSecurity, pending.SnapRefreshable)
}

// collectPending counts pending updates from apt and snap. Read-only.
func collectPending() Pending {
	p := Pending{}

	if _, err := exec.LookPath("apt"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		out, _ := exec.CommandContext(ctx, "apt", "list", "--upgradable").Output()
		s := bufio.NewScanner(strings.NewReader(string(out)))
		s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for s.Scan() {
			line := s.Text()
			// Header: "Listing... Done"; data rows: "name/archive[,archive2] version arch [upgradable from: ...]".
			if line == "" || strings.HasPrefix(line, "Listing") {
				continue
			}
			slash := strings.IndexByte(line, '/')
			space := strings.IndexByte(line, ' ')
			if slash <= 0 || space <= slash {
				continue
			}
			p.AptUpgradable++
			archives := line[slash+1 : space]
			if strings.Contains(archives, "-security") {
				p.AptSecurity++
			}
		}
	}

	if _, err := exec.LookPath("snap"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		// `snap refresh --list` prints "All snaps up to date." or a header + one row per refreshable snap.
		out, _ := exec.CommandContext(ctx, "snap", "refresh", "--list", "--unicode=never").Output()
		s := bufio.NewScanner(strings.NewReader(string(out)))
		first := true
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "All snaps up to date") {
				break
			}
			if first {
				first = false
				continue // header row
			}
			p.SnapRefreshable++
		}
	}

	return p
}

func execScript(ch *amqp.Channel, reportQueue, vmName, operationID, script string) {
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	output, err := cmd.CombinedOutput()

	status := "ok"
	exitCode := 0
	if err != nil {
		status = "failed"
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	publish(ch, reportQueue, Report{
		VMName:      vmName,
		Type:        "exec.result",
		OperationId: operationID,
		Status:      status,
		Output:      truncate(string(output), 4096),
		ExitCode:    exitCode,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})
}

func runFacts(ch *amqp.Channel, reportQueue, vmName string) {
	f := collectFacts()
	publish(ch, reportQueue, Report{
		VMName:       vmName,
		Type:         "facts.done",
		Status:       "ok",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		AgentVersion: agentVersion,
		Facts:        &f,
	})
	log.Printf("facts probe: cpu=%.1f%% mem=%.1f%% disk=%.1f%% uptime=%ds",
		f.CPUPercent, f.MemPercent, f.DiskPercent, f.UptimeSec)
}

func collectFacts() Facts {
	f := Facts{AgentVersion: agentVersion}

	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		f.Kernel = strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				f.OSPretty = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
		}
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if parts := strings.Fields(string(b)); len(parts) > 0 {
			if v, err := strconv.ParseFloat(parts[0], 64); err == nil {
				f.UptimeSec = int64(v)
			}
		}
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if parts := strings.Fields(string(b)); len(parts) > 0 {
			if v, err := strconv.ParseFloat(parts[0], 64); err == nil {
				f.LoadAvg1 = v
			}
		}
	}
	if c, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		f.CPUCount = strings.Count(string(c), "processor\t:")
	}
	f.CPUPercent = sampleCPUPercent()

	if mt, mu, err := readMemInfo(); err == nil {
		f.MemTotalKB = mt
		f.MemUsedKB = mu
		if mt > 0 {
			f.MemPercent = float64(mu) / float64(mt) * 100
		}
	}
	if dt, du, err := readDiskUsage("/"); err == nil {
		f.DiskTotalKB = dt
		f.DiskUsedKB = du
		if dt > 0 {
			f.DiskPercent = float64(du) / float64(dt) * 100
		}
	}
	f.PrimaryIP = primaryIP()
	return f
}

func sampleCPUPercent() float64 {
	idle1, total1, ok1 := readCPUTimes()
	if !ok1 {
		return 0
	}
	time.Sleep(200 * time.Millisecond)
	idle2, total2, ok2 := readCPUTimes()
	if !ok2 || total2 <= total1 {
		return 0
	}
	idleDelta := float64(idle2 - idle1)
	totalDelta := float64(total2 - total1)
	return (1.0 - idleDelta/totalDelta) * 100
}

func readCPUTimes() (idle, total uint64, ok bool) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "cpu ") {
			parts := strings.Fields(line)[1:]
			for i, p := range parts {
				v, err := strconv.ParseUint(p, 10, 64)
				if err != nil {
					return 0, 0, false
				}
				total += v
				if i == 3 {
					idle = v
				}
			}
			return idle, total, true
		}
	}
	return 0, 0, false
}

func readMemInfo() (totalKB, usedKB int64, err error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	var total, available int64
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		v, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		switch parts[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			available = v
		}
	}
	if total == 0 {
		return 0, 0, fmt.Errorf("no MemTotal")
	}
	return total, total - available, nil
}

func readDiskUsage(path string) (totalKB, usedKB int64, err error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return 0, 0, err
	}
	// blocks * block size → bytes; divide by 1024 for KB.
	total := int64(s.Blocks) * int64(s.Bsize) / 1024
	free := int64(s.Bavail) * int64(s.Bsize) / 1024
	return total, total - free, nil
}

func primaryIP() string {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if local, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return local.IP.String()
	}
	return ""
}

func heartbeat(ch *amqp.Channel, reportQueue, vmName string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		publish(ch, reportQueue, Report{
			VMName:    vmName,
			Type:      "heartbeat",
			Status:    "ok",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func loadConfig() Config {
	rabbitURL := os.Getenv("SUMMON_RABBIT_URL")
	if rabbitURL == "" {
		host := getEnv("SUMMON_RABBIT_HOST", "rabbitmq.rabbitmq.svc.cluster.local")
		user := getEnv("SUMMON_RABBIT_USER", "aegis")
		pass := getEnv("SUMMON_RABBIT_PASS", "guest")
		rabbitURL = fmt.Sprintf("amqp://%s:%s@%s:5672/",
			url.QueryEscape(user), url.QueryEscape(pass), host)
	}

	vmName := os.Getenv("SUMMON_VM_NAME")
	if vmName == "" {
		hostname, _ := os.Hostname()
		vmName = hostname
	}

	return Config{
		RabbitURL: rabbitURL,
		VMName:    vmName,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func connectRabbit(url string) (*amqp.Connection, *amqp.Channel) {
	// Infinite retry with capped exponential backoff. A cold-boot VM can hit Rabbit before
	// DNS, NetworkPolicies, or Rabbit itself are ready; there's no scenario where giving up
	// helps, and systemd `Restart=always` on a fatal exit just masked the real failure.
	attempt := 0
	delay := 2 * time.Second
	const maxDelay = 30 * time.Second

	for {
		attempt++
		conn, err := amqp.Dial(url)
		if err == nil {
			ch, cherr := conn.Channel()
			if cherr != nil {
				log.Printf("rabbitmq channel open failed (attempt %d): %v", attempt, cherr)
				conn.Close()
			} else {
				log.Printf("connected to rabbitmq after %d attempt(s)", attempt)
				return conn, ch
			}
		} else {
			log.Printf("rabbitmq connect attempt %d: %v (retrying in %s)", attempt, err, delay)
		}
		time.Sleep(delay)
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func declareQueue(ch *amqp.Channel, name string) {
	_, err := ch.QueueDeclare(name, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("failed to declare queue %s: %v", name, err)
	}
}

func publish(ch *amqp.Channel, queue string, report Report) {
	body, _ := json.Marshal(report)
	err := ch.Publish("", queue, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
	if err != nil {
		log.Printf("failed to publish to %s: %v", queue, err)
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "...(truncated)"
	}
	return s
}
