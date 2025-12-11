package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/aea/internal/apm"
	"github.com/example/aea/internal/collector"
	"github.com/example/aea/internal/config"
	"github.com/example/aea/internal/runtime"
	"github.com/example/aea/internal/sender"
	"github.com/example/aea/internal/telemetry"
)

func main() {
	cfgPath := flag.String("config", "aea.local.json", "path to local agent config")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	localCfg, err := config.LoadLocal(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	remoteCfg, err := config.FetchRemote(ctx, localCfg.RemoteConfigURL, localCfg.APIKey)
	if err != nil {
		log.Printf("remote config unavailable: %v", err)
	}

	effectiveCfg := config.EffectiveConfig{Local: localCfg, Remote: remoteCfg}

	queueSize := localCfg.QueueSize
	if queueSize <= 0 {
		queueSize = 1024
	}
	queue := telemetry.NewQueue(queueSize)

	httpSender, err := sender.NewHTTPSender(sender.HTTPConfig{
		Endpoint:    localCfg.CollectorEndpoint,
		Timeout:     10 * time.Second,
		MaxRetries:  4,
		Backoff:     750 * time.Millisecond,
		InsecureTLS: localCfg.TLSInsecure,
	})
	if err != nil {
		log.Fatalf("sender init: %v", err)
	}

	hostCollector := collector.NewHostMetricsCollector(10*time.Second, localCfg.MonitoredProcesses)
	collectors := []collector.Collector{hostCollector}
	appDiscovery := collector.NewApplicationDiscoveryCollector(30*time.Second, localCfg.Environment, localCfg.AppDiscoveryPaths)
	collectors = append(collectors, appDiscovery)
	connMapper := collector.NewConnectionMappingCollector(15*time.Second, localCfg.Environment, localCfg.AppDiscoveryPaths)
	collectors = append(collectors, connMapper)
	apmCollector := apm.NewCollector(apm.DefaultAggregator(), time.Minute)
	collectors = append(collectors, apmCollector)

	rt := runtime.NewRuntime(collectors, queue, httpSender, effectiveCfg)
	if err := rt.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("runtime stopped: %v", err)
	}
}
