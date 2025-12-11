package collector

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// HostMetrics represents a host-level sample.
type HostMetrics struct {
	Hostname  string           `json:"hostname"`
	Platform  string           `json:"platform"`
	Timestamp int64            `json:"timestamp"`
	CPU       CPUStat          `json:"cpu"`
	Memory    MemoryStat       `json:"memory"`
	Disk      DiskStat         `json:"disk"`
	Network   []NetworkStat    `json:"network"`
	Processes []ProcessMetrics `json:"processes"`
}

// CPUStat captures aggregate CPU usage.
type CPUStat struct {
	UsagePercent float64 `json:"usage_percent"`
	Load1        float64 `json:"load_1"`
	Load5        float64 `json:"load_5"`
	Load15       float64 `json:"load_15"`
}

// MemoryStat captures host memory usage.
type MemoryStat struct {
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	UsedPct    float64 `json:"used_percent"`
}

// DiskStat captures root filesystem usage.
type DiskStat struct {
	Path       string  `json:"path"`
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	UsedPct    float64 `json:"used_percent"`
}

// NetworkStat captures per-interface network throughput counters.
type NetworkStat struct {
	Interface  string `json:"interface"`
	BytesSent  uint64 `json:"bytes_sent"`
	BytesRecv  uint64 `json:"bytes_recv"`
	PacketsIn  uint64 `json:"packets_in"`
	PacketsOut uint64 `json:"packets_out"`
}

// ProcessMetrics represents CPU and memory for a monitored process.
type ProcessMetrics struct {
	Name        string  `json:"name"`
	PID         int32   `json:"pid"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes uint64  `json:"memory_bytes"`
	MemoryPct   float32 `json:"memory_percent"`
}

// HostMetricsCollector collects CPU, memory, disk, network, and selected process metrics.
type HostMetricsCollector struct {
	name      string
	interval  int64
	processes map[string]struct{}
}

// NewHostMetricsCollector builds a host metrics collector.
func NewHostMetricsCollector(interval time.Duration, monitoredProcesses []string) *HostMetricsCollector {
	procSet := make(map[string]struct{}, len(monitoredProcesses))
	for _, p := range monitoredProcesses {
		procSet[p] = struct{}{}
	}

	return &HostMetricsCollector{
		name:      "host_metrics",
		interval:  interval.Milliseconds(),
		processes: procSet,
	}
}

func (h *HostMetricsCollector) Name() string    { return h.name }
func (h *HostMetricsCollector) Interval() int64 { return h.interval }

// Collect gathers host and process metrics using OS-native providers.
func (h *HostMetricsCollector) Collect(ctx context.Context) (Payload, error) {
	now := time.Now().UnixMilli()

	hostInfo, _ := host.InfoWithContext(ctx)
	hostname := hostInfo.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	cpuPct := sampleCPUPct(ctx)
	load1, load5, load15 := sampleLoad(ctx)
	memStat := sampleMemory(ctx)
	diskStat := sampleDisk(ctx)
	netStat := sampleNetwork(ctx)
	procs := h.sampleProcesses(ctx)

	sample := HostMetrics{
		Hostname:  hostname,
		Platform:  hostInfo.Platform,
		Timestamp: now,
		CPU: CPUStat{
			UsagePercent: cpuPct,
			Load1:        load1,
			Load5:        load5,
			Load15:       load15,
		},
		Memory:    memStat,
		Disk:      diskStat,
		Network:   netStat,
		Processes: procs,
	}

	data, err := json.Marshal(sample)
	if err != nil {
		return Payload{}, err
	}

	return Payload{
		Name:      h.name,
		Timestamp: now,
		Data:      data,
	}, nil
}

func sampleCPUPct(ctx context.Context) float64 {
	pct, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil || len(pct) == 0 {
		return 0
	}
	return pct[0]
}

func sampleLoad(ctx context.Context) (float64, float64, float64) {
	avg, err := cpu.LoadAvgWithContext(ctx)
	if err != nil || avg == nil {
		return 0, 0, 0
	}
	return avg.Load1, avg.Load5, avg.Load15
}

func sampleMemory(ctx context.Context) MemoryStat {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil || vm == nil {
		return MemoryStat{}
	}
	return MemoryStat{
		TotalBytes: vm.Total,
		UsedBytes:  vm.Used,
		UsedPct:    vm.UsedPercent,
	}
}

func sampleDisk(ctx context.Context) DiskStat {
	path := "/"
	if runtime.GOOS == "windows" {
		path = "C:\\"
	}
	usage, err := disk.UsageWithContext(ctx, path)
	if err != nil || usage == nil {
		return DiskStat{Path: path}
	}
	return DiskStat{
		Path:       path,
		TotalBytes: usage.Total,
		UsedBytes:  usage.Used,
		UsedPct:    usage.UsedPercent,
	}
}

func sampleNetwork(ctx context.Context) []NetworkStat {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil || len(counters) == 0 {
		return nil
	}
	stats := make([]NetworkStat, 0, len(counters))
	for _, c := range counters {
		stats = append(stats, NetworkStat{
			Interface:  c.Name,
			BytesSent:  c.BytesSent,
			BytesRecv:  c.BytesRecv,
			PacketsIn:  c.PacketsRecv,
			PacketsOut: c.PacketsSent,
		})
	}
	return stats
}

func (h *HostMetricsCollector) sampleProcesses(ctx context.Context) []ProcessMetrics {
	if len(h.processes) == 0 {
		return nil
	}

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil
	}

	samples := make([]ProcessMetrics, 0, len(h.processes))
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		if _, ok := h.processes[name]; !ok {
			continue
		}

		cpuPct, _ := p.CPUPercentWithContext(ctx)
		memInfo, _ := p.MemoryInfoWithContext(ctx)
		memPct, _ := p.MemoryPercentWithContext(ctx)

		samples = append(samples, ProcessMetrics{
			Name:       name,
			PID:        p.Pid,
			CPUPercent: cpuPct,
			MemoryBytes: func() uint64 {
				if memInfo != nil {
					return memInfo.RSS
				}
				return 0
			}(),
			MemoryPct: memPct,
		})
	}
	return samples
}

// ExampleHostMetricsPayload demonstrates the JSON payload shape sent to the Collector.
var ExampleHostMetricsPayload = `{
  "hostname": "edge-node-01",
  "platform": "linux",
  "timestamp": 1712345678901,
  "cpu": {"usage_percent": 12.5, "load_1": 0.32, "load_5": 0.44, "load_15": 0.41},
  "memory": {"total_bytes": 16755277824, "used_bytes": 8246337208, "used_percent": 49.2},
  "disk": {"path": "/", "total_bytes": 512110190592, "used_bytes": 233646383104, "used_percent": 45.6},
  "network": [
    {"interface": "eth0", "bytes_sent": 1234567, "bytes_recv": 7654321, "packets_in": 1024, "packets_out": 2048}
  ],
  "processes": [
    {"name": "postgres", "pid": 4242, "cpu_percent": 3.1, "memory_bytes": 268435456, "memory_percent": 1.6},
    {"name": "nginx", "pid": 1337, "cpu_percent": 1.2, "memory_bytes": 134217728, "memory_percent": 0.8}
  ]
}`
