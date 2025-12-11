package collector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ApplicationRecord captures a discovered application on the host.
type ApplicationRecord struct {
	ApplicationID  string            `json:"application_id"`
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Type           string            `json:"type"`
	ExecutablePath string            `json:"executable_path"`
	Owner          string            `json:"owner"`
	Environment    string            `json:"environment"`
	Tags           map[string]string `json:"tags,omitempty"`
}

// ApplicationInventory is the payload envelope for discovery results.
type ApplicationInventory struct {
	Hostname     string              `json:"hostname"`
	Environment  string              `json:"environment"`
	Timestamp    int64               `json:"timestamp"`
	Applications []ApplicationRecord `json:"applications"`
}

// ApplicationDiscoveryCollector inspects the host for running applications and services.
type ApplicationDiscoveryCollector struct {
	name       string
	interval   int64
	env        string
	knownPaths []string
}

// NewApplicationDiscoveryCollector creates a collector with the provided interval and environment label.
func NewApplicationDiscoveryCollector(interval time.Duration, env string, knownPaths []string) *ApplicationDiscoveryCollector {
	if len(knownPaths) == 0 {
		knownPaths = []string{filepath.Join(string(os.PathSeparator), "opt"), filepath.Join(string(os.PathSeparator), "usr", "local", "bin")}
	}
	return &ApplicationDiscoveryCollector{
		name:       "app_discovery",
		interval:   interval.Milliseconds(),
		env:        env,
		knownPaths: knownPaths,
	}
}

func (a *ApplicationDiscoveryCollector) Name() string    { return a.name }
func (a *ApplicationDiscoveryCollector) Interval() int64 { return a.interval }

// Collect inspects OS-specific sources and emits an application inventory payload.
func (a *ApplicationDiscoveryCollector) Collect(ctx context.Context) (Payload, error) {
	now := time.Now().UnixMilli()
	hostname, _ := os.Hostname()

	apps := platformDiscoverApplications(ctx, a.env, a.knownPaths)

	payload := ApplicationInventory{
		Hostname:     hostname,
		Environment:  a.env,
		Timestamp:    now,
		Applications: apps,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return Payload{}, err
	}

	return Payload{
		Name:      a.name,
		Timestamp: now,
		Data:      data,
	}, nil
}

func discoverKnownDirectories(path, env string) []ApplicationRecord {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}

	records := make([]ApplicationRecord, 0, len(entries))
	for _, entry := range entries {
		record := ApplicationRecord{
			ApplicationID:  filepath.Join(path, entry.Name()),
			Name:           entry.Name(),
			Type:           "directory_app",
			ExecutablePath: filepath.Join(path, entry.Name()),
			Environment:    env,
		}
		records = append(records, record)
	}
	return records
}

// ExampleAppDiscoveryPayload demonstrates the JSON payload shape sent to the Collector.
var ExampleAppDiscoveryPayload = `{
  "hostname": "edge-node-01",
  "environment": "prod",
  "timestamp": 1712345678901,
  "applications": [
    {"application_id": "sshd.service", "name": "sshd.service", "version": "", "type": "systemd_service", "executable_path": "/usr/sbin/sshd", "owner": "root", "environment": "prod", "tags": {"state": "running"}},
    {"application_id": "nginx", "name": "nginx", "version": "1.24.0", "type": "process", "executable_path": "/usr/sbin/nginx", "owner": "root", "environment": "prod"},
    {"application_id": "site-Default Web Site", "name": "Default Web Site", "version": "", "type": "iis_site", "executable_path": "C:\\Windows\\System32\\inetsrv\\w3wp.exe", "owner": "IIS APPPOOL\\DefaultAppPool", "environment": "prod", "tags": {"bindings": "http/*:80:"}}
  ]
}`
