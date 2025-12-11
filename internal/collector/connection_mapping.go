package collector

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	gnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// ConnectionDependency links a discovered application to a remote endpoint.
type ConnectionDependency struct {
	ApplicationID   string            `json:"application_id"`
	ApplicationName string            `json:"application_name"`
	ProcessID       int32             `json:"process_id"`
	ExecutablePath  string            `json:"executable_path"`
	Protocol        string            `json:"protocol"`
	LocalAddress    string            `json:"local_address"`
	LocalPort       uint32            `json:"local_port"`
	RemoteAddress   string            `json:"remote_address"`
	RemotePort      uint32            `json:"remote_port"`
	DependencyType  string            `json:"dependency_type"`
	DatabaseHint    string            `json:"database_hint,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
}

// ConnectionMappingPayload is the envelope emitted by the connection mapping collector.
type ConnectionMappingPayload struct {
	Hostname     string                 `json:"hostname"`
	Environment  string                 `json:"environment"`
	Timestamp    int64                  `json:"timestamp"`
	Dependencies []ConnectionDependency `json:"dependencies"`
}

// ConnectionMappingCollector inspects active socket connections and ties them to applications.
type ConnectionMappingCollector struct {
	name       string
	interval   int64
	env        string
	knownPaths []string
}

// NewConnectionMappingCollector builds a collector that samples connections periodically.
func NewConnectionMappingCollector(interval time.Duration, env string, knownPaths []string) *ConnectionMappingCollector {
	if len(knownPaths) == 0 {
		knownPaths = []string{filepath.Join(string(os.PathSeparator), "opt"), filepath.Join(string(os.PathSeparator), "usr", "local", "bin")}
	}
	return &ConnectionMappingCollector{
		name:       "connection_mapping",
		interval:   interval.Milliseconds(),
		env:        env,
		knownPaths: knownPaths,
	}
}

func (c *ConnectionMappingCollector) Name() string    { return c.name }
func (c *ConnectionMappingCollector) Interval() int64 { return c.interval }

// Collect snapshots current TCP/UDP connections and maps them to discovered applications.
func (c *ConnectionMappingCollector) Collect(ctx context.Context) (Payload, error) {
	now := time.Now().UnixMilli()
	hostname, _ := os.Hostname()

	apps := platformDiscoverApplications(ctx, c.env, c.knownPaths)
	idx := buildAppIndex(apps)

	conns, err := gnet.ConnectionsWithContext(ctx, "inet")
	if err != nil {
		return Payload{}, err
	}

	procCache := make(map[int32]processIdentity)
	dependencies := make([]ConnectionDependency, 0, len(conns))

	for _, conn := range conns {
		if conn.Raddr.IP == "" || conn.Raddr.Port == 0 {
			continue // skip listeners
		}
		if conn.Status != "ESTABLISHED" && conn.Type != syscall.SOCK_DGRAM {
			continue
		}

		pid := conn.Pid
		if pid == 0 {
			continue
		}

		procInfo, ok := procCache[pid]
		if !ok {
			p, err := process.NewProcessWithContext(ctx, pid)
			if err != nil {
				continue
			}
			name, _ := p.NameWithContext(ctx)
			exe, _ := p.ExeWithContext(ctx)
			owner, _ := p.UsernameWithContext(ctx)
			procInfo = processIdentity{PID: pid, Name: name, Exe: exe, Owner: owner}
			procCache[pid] = procInfo
		}

		app := idx.find(procInfo, c.env)
		depType, dbHint := classifyDependency(conn.Raddr.IP, conn.Raddr.Port)

		dep := ConnectionDependency{
			ApplicationID:   app.ApplicationID,
			ApplicationName: app.Name,
			ProcessID:       pid,
			ExecutablePath:  procInfo.Exe,
			Protocol:        protocolName(conn.Type),
			LocalAddress:    conn.Laddr.IP,
			LocalPort:       conn.Laddr.Port,
			RemoteAddress:   conn.Raddr.IP,
			RemotePort:      conn.Raddr.Port,
			DependencyType:  depType,
			DatabaseHint:    dbHint,
			Tags:            map[string]string{"status": conn.Status},
		}
		if dep.Tags["status"] == "" {
			delete(dep.Tags, "status")
		}

		dependencies = append(dependencies, dep)
	}

	payload := ConnectionMappingPayload{
		Hostname:     hostname,
		Environment:  c.env,
		Timestamp:    now,
		Dependencies: dependencies,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return Payload{}, err
	}

	return Payload{Name: c.name, Timestamp: now, Data: data}, nil
}

type processIdentity struct {
	PID   int32
	Name  string
	Exe   string
	Owner string
}

type appIndex struct {
	apps   []ApplicationRecord
	byExe  map[string]int
	byName map[string]int
	byID   map[string]int
}

func buildAppIndex(apps []ApplicationRecord) appIndex {
	idx := appIndex{
		apps:   apps,
		byExe:  make(map[string]int),
		byName: make(map[string]int),
		byID:   make(map[string]int),
	}
	for i := range apps {
		rec := apps[i]
		if rec.ExecutablePath != "" {
			idx.byExe[strings.ToLower(rec.ExecutablePath)] = i
		}
		if rec.Name != "" {
			idx.byName[strings.ToLower(rec.Name)] = i
		}
		if rec.ApplicationID != "" {
			idx.byID[strings.ToLower(rec.ApplicationID)] = i
		}
	}
	return idx
}

func (idx appIndex) find(proc processIdentity, env string) ApplicationRecord {
	if pos, ok := idx.byExe[strings.ToLower(proc.Exe)]; ok {
		return idx.apps[pos]
	}
	if pos, ok := idx.byName[strings.ToLower(proc.Name)]; ok {
		return idx.apps[pos]
	}
	if pos, ok := idx.byID[strings.ToLower(proc.Name)]; ok {
		return idx.apps[pos]
	}
	return ApplicationRecord{
		ApplicationID:  proc.Name,
		Name:           proc.Name,
		Type:           "process",
		ExecutablePath: proc.Exe,
		Owner:          proc.Owner,
		Environment:    env,
	}
}

func protocolName(sockType uint32) string {
	switch sockType {
	case syscall.SOCK_DGRAM:
		return "udp"
	default:
		return "tcp"
	}
}

func classifyDependency(remoteIP string, remotePort uint32) (string, string) {
	dbPorts := map[uint32]string{
		1433:  "sqlserver",
		5432:  "postgres",
		3306:  "mysql",
		1521:  "oracle",
		6379:  "redis",
		27017: "mongodb",
		9200:  "elasticsearch",
	}
	if db, ok := dbPorts[remotePort]; ok {
		return "database", db
	}

	host := strings.ToLower(remoteIP)
	if strings.Contains(host, "sql") || strings.Contains(host, "db") || strings.Contains(host, "postgres") || strings.Contains(host, "mysql") {
		return "database", "heuristic"
	}

	if net.ParseIP(remoteIP) != nil && (strings.HasPrefix(remoteIP, "10.") || strings.HasPrefix(remoteIP, "192.168.")) {
		return "external_service", ""
	}

	return "external_service", ""
}

// ExampleConnectionMappingPayload demonstrates the JSON payload shape sent to the Collector.
var ExampleConnectionMappingPayload = `{
  "hostname": "edge-node-01",
  "environment": "prod",
  "timestamp": 1712345678901,
  "dependencies": [
    {"application_id": "postgres", "application_name": "postgres", "process_id": 4242, "executable_path": "/usr/lib/postgresql/15/bin/postgres", "protocol": "tcp", "local_address": "10.0.0.5", "local_port": 55432, "remote_address": "10.0.0.10", "remote_port": 5432, "dependency_type": "database", "database_hint": "postgres", "tags": {"status": "ESTABLISHED"}},
    {"application_id": "nginx", "application_name": "nginx", "process_id": 1337, "executable_path": "/usr/sbin/nginx", "protocol": "tcp", "local_address": "10.0.0.5", "local_port": 443, "remote_address": "52.22.11.9", "remote_port": 443, "dependency_type": "external_service"}
  ]
}`
