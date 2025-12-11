//go:build linux
// +build linux

package collector

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

func platformDiscoverApplications(ctx context.Context, env string, knownPaths []string) []ApplicationRecord {
	records := make([]ApplicationRecord, 0)

	// Running systemd services
	records = append(records, discoverSystemdServices(ctx, env)...)

	// Running processes
	records = append(records, discoverProcesses(ctx, env)...)

	// Known application directories
	for _, path := range knownPaths {
		records = append(records, discoverKnownDirectories(path, env)...)
	}

	return records
}

func discoverSystemdServices(ctx context.Context, env string) []ApplicationRecord {
	cmd := exec.CommandContext(ctx, "systemctl", "list-units", "--type=service", "--state=running", "--no-legend", "--no-pager", "--plain")
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	records := make([]ApplicationRecord, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		svc := fields[0]
		record := ApplicationRecord{
			ApplicationID: svc,
			Name:          svc,
			Type:          "systemd_service",
			Environment:   env,
			Owner:         "systemd",
			Tags:          map[string]string{"state": "running"},
		}
		records = append(records, record)
	}
	return records
}

func discoverProcesses(ctx context.Context, env string) []ApplicationRecord {
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
