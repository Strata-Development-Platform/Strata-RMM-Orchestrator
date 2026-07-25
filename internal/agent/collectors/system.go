package collectors

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/core"
)

type SystemCollector struct {
	interval time.Duration
}

func NewSystemCollector(interval time.Duration) *SystemCollector {
	return &SystemCollector{interval: interval}
}

func (c *SystemCollector) Name() string { return "system" }

func (c *SystemCollector) Interval() time.Duration { return c.interval }

func (c *SystemCollector) Start(ctx context.Context) error { return nil }

func (c *SystemCollector) Stop() error { return nil }

func (c *SystemCollector) Collect(ctx context.Context) ([]core.MetricSample, error) {
	var samples []core.MetricSample
	now := time.Now()

	hostInfo, err := host.Info()
	if err == nil {
		samples = append(samples,
			core.MetricSample{Name: "system.uptime", Value: float64(hostInfo.Uptime), Timestamp: now},
		)
	}

	cpuPercent, err := cpu.PercentWithContext(ctx, 0, false)
	if err == nil && len(cpuPercent) > 0 {
		samples = append(samples, core.MetricSample{Name: "cpu.percent", Value: cpuPercent[0], Timestamp: now})
	}

	cpuCount, _ := cpu.Counts(true)
	samples = append(samples, core.MetricSample{Name: "cpu.cores", Value: float64(cpuCount), Timestamp: now})

	loadAvg, err := load.AvgWithContext(ctx)
	if err == nil {
		samples = append(samples,
			core.MetricSample{Name: "load.1", Value: loadAvg.Load1, Timestamp: now},
			core.MetricSample{Name: "load.5", Value: loadAvg.Load5, Timestamp: now},
			core.MetricSample{Name: "load.15", Value: loadAvg.Load15, Timestamp: now},
		)
	}

	memInfo, err := mem.VirtualMemoryWithContext(ctx)
	if err == nil {
		samples = append(samples,
			core.MetricSample{Name: "memory.total", Value: float64(memInfo.Total), Tags: map[string]string{"unit": "bytes"}, Timestamp: now},
			core.MetricSample{Name: "memory.used", Value: float64(memInfo.Used), Tags: map[string]string{"unit": "bytes"}, Timestamp: now},
			core.MetricSample{Name: "memory.available", Value: float64(memInfo.Available), Tags: map[string]string{"unit": "bytes"}, Timestamp: now},
			core.MetricSample{Name: "memory.percent", Value: memInfo.UsedPercent, Timestamp: now},
		)
	}

	swapInfo, err := mem.SwapMemoryWithContext(ctx)
	if err == nil {
		samples = append(samples,
			core.MetricSample{Name: "swap.total", Value: float64(swapInfo.Total), Tags: map[string]string{"unit": "bytes"}, Timestamp: now},
			core.MetricSample{Name: "swap.used", Value: float64(swapInfo.Used), Tags: map[string]string{"unit": "bytes"}, Timestamp: now},
			core.MetricSample{Name: "swap.percent", Value: swapInfo.UsedPercent, Timestamp: now},
		)
	}

	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err == nil {
		for _, p := range partitions {
			usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
			if err != nil {
				continue
			}
			tags := map[string]string{"mount": p.Mountpoint, "fstype": p.Fstype, "device": p.Device}
			samples = append(samples,
				core.MetricSample{Name: "disk.total", Value: float64(usage.Total), Tags: tags, Timestamp: now},
				core.MetricSample{Name: "disk.used", Value: float64(usage.Used), Tags: tags, Timestamp: now},
				core.MetricSample{Name: "disk.free", Value: float64(usage.Free), Tags: tags, Timestamp: now},
				core.MetricSample{Name: "disk.percent", Value: usage.UsedPercent, Tags: tags, Timestamp: now},
			)
		}
	}

	netIO, err := net.IOCountersWithContext(ctx, true)
	if err == nil {
		for _, n := range netIO {
			tags := map[string]string{"interface": n.Name}
			samples = append(samples,
				core.MetricSample{Name: "net.bytes_sent", Value: float64(n.BytesSent), Tags: tags, Timestamp: now},
				core.MetricSample{Name: "net.bytes_recv", Value: float64(n.BytesRecv), Tags: tags, Timestamp: now},
				core.MetricSample{Name: "net.packets_sent", Value: float64(n.PacketsSent), Tags: tags, Timestamp: now},
				core.MetricSample{Name: "net.packets_recv", Value: float64(n.PacketsRecv), Tags: tags, Timestamp: now},
				core.MetricSample{Name: "net.errors_in", Value: float64(n.Errin), Tags: tags, Timestamp: now},
				core.MetricSample{Name: "net.errors_out", Value: float64(n.Errout), Tags: tags, Timestamp: now},
			)
		}
	}

	goarch := runtime.GOARCH
	goos := runtime.GOOS
	samples = append(samples,
		core.MetricSample{Name: "system.info", Value: 1, Tags: map[string]string{
			"os": goos, "arch": goarch, "hostname": hostInfo.Hostname,
			"platform": hostInfo.Platform, "platform_version": hostInfo.PlatformVersion,
		}, Timestamp: now},
	)

	return samples, nil
}

func (c *SystemCollector) collectDiskIO(ctx context.Context) ([]core.MetricSample, error) {
	diskIO, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("disk IO: %w", err)
	}
	var samples []core.MetricSample
	now := time.Now()
	for name, io := range diskIO {
		tags := map[string]string{"disk": name}
		samples = append(samples,
			core.MetricSample{Name: "diskio.read_count", Value: float64(io.ReadCount), Tags: tags, Timestamp: now},
			core.MetricSample{Name: "diskio.write_count", Value: float64(io.WriteCount), Tags: tags, Timestamp: now},
			core.MetricSample{Name: "diskio.read_bytes", Value: float64(io.ReadBytes), Tags: tags, Timestamp: now},
			core.MetricSample{Name: "diskio.write_bytes", Value: float64(io.WriteBytes), Tags: tags, Timestamp: now},
			core.MetricSample{Name: "diskio.iops_in_progress", Value: float64(io.IopsInProgress), Tags: tags, Timestamp: now},
		)
	}
	return samples, nil
}
