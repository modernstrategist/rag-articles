package config

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"
)

// LocalConfig represents settings persisted on disk.
type LocalConfig struct {
	CollectorEndpoint  string           `json:"collector_endpoint"`
	RemoteConfigURL    string           `json:"remote_config_url"`
	APIKey             string           `json:"api_key"`
	Environment        string           `json:"environment"`
	QueueSize          int              `json:"queue_size"`
	BatchSize          int              `json:"batch_size"`
	BatchIntervalMS    int              `json:"batch_interval_ms"`
	CollectorWindows   map[string]int64 `json:"collector_windows_ms"`
	MonitoredProcesses []string         `json:"monitored_processes"`
	AppDiscoveryPaths  []string         `json:"app_discovery_paths"`
	TLSInsecure        bool             `json:"tls_insecure"`
}

// RemoteConfig augments or overrides local settings pulled from the Collector API.
type RemoteConfig struct {
	CollectorWindows map[string]int64 `json:"collector_windows_ms"`
	FeatureFlags     map[string]bool  `json:"feature_flags"`
}

// EffectiveConfig merges local and remote configuration.
type EffectiveConfig struct {
	Local  LocalConfig
	Remote RemoteConfig
}

// LoadLocal reads a JSON configuration file from disk.
func LoadLocal(path string) (LocalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LocalConfig{}, err
	}

	var cfg LocalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return LocalConfig{}, err
	}
	return cfg, nil
}

// FetchRemote retrieves collector configuration from a remote endpoint.
func FetchRemote(ctx context.Context, url, apiKey string) (RemoteConfig, error) {
	if url == "" {
		return RemoteConfig{}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return RemoteConfig{}, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return RemoteConfig{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return RemoteConfig{}, errors.New(resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RemoteConfig{}, err
	}

	var cfg RemoteConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return RemoteConfig{}, err
	}

	return cfg, nil
}

// ResolveWindow returns the interval for a given collector name.
func (e EffectiveConfig) ResolveWindow(name string, defaultMS int64) time.Duration {
	if v, ok := e.Remote.CollectorWindows[name]; ok && v > 0 {
		return time.Duration(v) * time.Millisecond
	}
	if v, ok := e.Local.CollectorWindows[name]; ok && v > 0 {
		return time.Duration(v) * time.Millisecond
	}
	return time.Duration(defaultMS) * time.Millisecond
}
