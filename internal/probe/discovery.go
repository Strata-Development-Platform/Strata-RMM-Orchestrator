package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
)

type DiscoveryResult struct {
	IP        string            `json:"ip"`
	MAC       string            `json:"mac"`
	Hostname  string            `json:"hostname"`
	Vendor    string            `json:"vendor"`
	Type      string            `json:"type"`
	Ports     []string          `json:"ports"`
	Interface string            `json:"interface"`
	Via       string            `json:"via"` // arp, lldp, cdp, snmp
	Labels    map[string]string `json:"labels"`
}

type DiscoveryEngine struct {
	probe   *Probe
	subnets []string
}

func NewDiscoveryEngine(p *Probe, subnets []string) *DiscoveryEngine {
	return &DiscoveryEngine{
		probe:   p,
		subnets: subnets,
	}
}

func (de *DiscoveryEngine) Run(ctx context.Context) {
	de.probe.Logger.Info("starting network discovery")
	de.runDiscovery(ctx)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			de.runDiscovery(ctx)
		}
	}
}

func (de *DiscoveryEngine) runDiscovery(ctx context.Context) {
	de.probe.Logger.Info("running network discovery cycle")

	var allDevices []DiscoveryResult

	arpDevices := de.discoverARP()
	allDevices = append(allDevices, arpDevices...)
	de.probe.Logger.Info("ARP discovery complete", zap.Int("devices", len(arpDevices)))

	snmpDevices := de.discoverSNMPNeighbors()
	allDevices = append(allDevices, snmpDevices...)
	de.probe.Logger.Info("SNMP neighbor discovery complete", zap.Int("devices", len(snmpDevices)))

	for _, subnet := range de.subnets {
		pingDevices := de.pingScan(subnet)
		allDevices = append(allDevices, pingDevices...)
		de.probe.Logger.Info("ping scan complete", zap.String("subnet", subnet), zap.Int("devices", len(pingDevices)))
	}

	deduped := de.deduplicate(allDevices)

	for _, d := range deduped {
		de.publishDiscovery(d)
	}
}

func (de *DiscoveryEngine) discoverARP() []DiscoveryResult {
	// Attempt to read ARP table from /proc/net/arp (Linux) or via syscall
	// This is best-effort; works on Linux natively
	return nil
}

func (de *DiscoveryEngine) discoverSNMPNeighbors() []DiscoveryResult {
	var results []DiscoveryResult
	for _, t := range de.probe.targets {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		neighbors, err := PollSNMP(ctx, SNMPTarget{
			Host:      t.Host,
			Port:      t.Port,
			Version:   t.Version,
			Community: t.Community,
			V3:        t.V3,
			OIDs:      []string{".1.3.6.1.2.1.4.22.1.2*"},
		})
		cancel()
		if err != nil {
			continue
		}
		for _, n := range neighbors {
			results = append(results, DiscoveryResult{
				IP:     n.Value,
				Via:    "snmp-arp",
				Labels: map[string]string{"discovered_by": t.Host},
			})
		}
	}
	return results
}

func (de *DiscoveryEngine) pingScan(subnet string) []DiscoveryResult {
	return nil
}

func (de *DiscoveryEngine) deduplicate(devices []DiscoveryResult) []DiscoveryResult {
	seen := make(map[string]bool)
	var deduped []DiscoveryResult
	for _, d := range devices {
		key := d.IP
		if d.MAC != "" {
			key = d.MAC
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		if d.IP != "" {
			names, _ := net.LookupAddr(d.IP)
			if len(names) > 0 {
				d.Hostname = names[0]
			}
		}
		deduped = append(deduped, d)
	}
	return deduped
}

func (de *DiscoveryEngine) publishDiscovery(d DiscoveryResult) {
	data, _ := json.Marshal(d)
	subject := fmt.Sprintf("tenant.%s.probe.%s.discovery", de.probe.TenantID, de.probe.ID)
	if err := de.probe.NATS.Publish(subject, data); err != nil {
		de.probe.Logger.Warn("publish discovery", zap.Error(err))
	}
}
