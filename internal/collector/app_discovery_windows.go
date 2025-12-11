//go:build windows
// +build windows

package collector

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/shirou/gopsutil/v3/winservices"
)

func platformDiscoverApplications(ctx context.Context, env string, knownPaths []string) []ApplicationRecord {
	records := make([]ApplicationRecord, 0)

	records = append(records, discoverWindowsServices(env)...)
	records = append(records, discoverIIS(env)...)
	records = append(records, discoverRunningProcesses(ctx, env)...)

	for _, path := range knownPaths {
		records = append(records, discoverKnownDirectories(path, env)...)
	}

	return records
}

func discoverWindowsServices(env string) []ApplicationRecord {
	svcs, err := winservices.ListServices()
	if err != nil {
		return nil
	}

	records := make([]ApplicationRecord, 0, len(svcs))
	for _, s := range svcs {
		service, err := s.Service()
		if err != nil {
			continue
		}
		cfg, _ := service.Config()
		status, _ := service.Status()

		record := ApplicationRecord{
			ApplicationID:  s.Name,
			Name:           s.DisplayName,
			Type:           "windows_service",
			ExecutablePath: cfg.BinaryPathName,
			Owner:          cfg.ServiceStartName,
			Environment:    env,
			Tags:           map[string]string{"state": status.State.String()},
		}
		records = append(records, record)
	}
	return records
}

// Minimal parsing of IIS sites from applicationHost.config.
type applicationHostConfig struct {
	Sites []iisSite `xml:"system.applicationHost>sites>site"`
}

type iisSite struct {
	ID       string      `xml:"id,attr"`
	Name     string      `xml:"name,attr"`
	Bindings []iisBind   `xml:"bindings>binding"`
	Apps     []iisAppDef `xml:"application"`
}

type iisBind struct {
	Binding string `xml:"bindingInformation,attr"`
}

type iisAppDef struct {
	Path string `xml:"path,attr"`
}

func discoverIIS(env string) []ApplicationRecord {
	configPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "inetsrv", "config", "applicationHost.config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var cfg applicationHostConfig
	if err := xml.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	records := make([]ApplicationRecord, 0, len(cfg.Sites))
	for _, site := range cfg.Sites {
		tagMap := make(map[string]string)
		if len(site.Bindings) > 0 {
			bindings := make([]string, 0, len(site.Bindings))
			for _, b := range site.Bindings {
				bindings = append(bindings, b.Binding)
			}
			tagMap["bindings"] = strings.Join(bindings, ",")
		}
		record := ApplicationRecord{
			ApplicationID:  "site-" + site.Name,
			Name:           site.Name,
			Type:           "iis_site",
			ExecutablePath: filepath.Join(os.Getenv("SystemRoot"), "System32", "inetsrv", "w3wp.exe"),
			Owner:          "IIS APPPOOL\\DefaultAppPool",
			Environment:    env,
			Tags:           tagMap,
		}
		records = append(records, record)
	}
	return records
}

func discoverRunningProcesses(ctx context.Context, env string) []ApplicationRecord {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil
	}

	records := make([]ApplicationRecord, 0, len(procs))
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		exe, _ := p.ExeWithContext(ctx)
		owner, _ := p.UsernameWithContext(ctx)
		record := ApplicationRecord{
			ApplicationID:  name,
			Name:           name,
			Type:           "process",
			ExecutablePath: exe,
			Owner:          owner,
			Environment:    env,
		}
		records = append(records, record)
	}
	return records
}
