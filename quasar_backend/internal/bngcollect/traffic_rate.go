package bngcollect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// TrafficRateSnapshot taxa instantânea calculada por delta Flow64.
type TrafficRateSnapshot struct {
	UpBps      int64  `json:"up_bps"`
	DnBps      int64  `json:"dn_bps"`
	UpDisplay  string `json:"up_display"`
	DnDisplay  string `json:"dn_display"`
	IntervalMs int64  `json:"interval_ms"`
	SampledAt  string `json:"sampled_at"`
}

const defaultTrafficSampleInterval = 1500 * time.Millisecond

// MeasureSessionFlow64Rate amostra hwAccess*Flow64 duas vezes e calcula bps.
func MeasureSessionFlow64Rate(ctx context.Context, host, community string, profile Profile, idx string, interval time.Duration) (TrafficRateSnapshot, error) {
	idx = strings.TrimSpace(idx)
	host = strings.TrimSpace(host)
	community = strings.TrimSpace(community)
	if idx == "" || host == "" || community == "" {
		return TrafficRateSnapshot{}, fmt.Errorf("índice ou SNMP em falta")
	}
	if interval <= 0 {
		interval = defaultTrafficSampleInterval
	}
	timeout := 12 * time.Second

	up1, dn1, err := readFlow64Counters(ctx, host, community, profile, idx, timeout)
	if err != nil {
		return TrafficRateSnapshot{}, err
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return TrafficRateSnapshot{}, ctx.Err()
	case <-timer.C:
	}
	up2, dn2, err := readFlow64Counters(ctx, host, community, profile, idx, timeout)
	if err != nil {
		return TrafficRateSnapshot{}, err
	}

	elapsed := interval.Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	upDelta := flow64Delta(up1, up2)
	dnDelta := flow64Delta(dn1, dn2)
	upBps := int64(float64(upDelta*64*8) / elapsed)
	dnBps := int64(float64(dnDelta*64*8) / elapsed)

	return TrafficRateSnapshot{
		UpBps:      upBps,
		DnBps:      dnBps,
		UpDisplay:  FormatBitrateBps(upBps),
		DnDisplay:  FormatBitrateBps(dnBps),
		IntervalMs: interval.Milliseconds(),
		SampledAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

type flow64OIDPair struct {
	up string
	dn string
}

func flow64OIDCandidates(profile Profile) []flow64OIDPair {
	up := metricBaseOID(profile, "access_up_flow")
	dn := metricBaseOID(profile, "access_dn_flow")
	out := []flow64OIDPair{{up: up, dn: dn}}
	// Fallbacks Huawei (hwAccessTable / hwAAAAccessTable / contadores alternativos).
	fallbacks := []flow64OIDPair{
		{"1.3.6.1.4.1.2011.5.2.1.15.1.36", "1.3.6.1.4.1.2011.5.2.1.15.1.37"},
		{"1.3.6.1.4.1.2011.5.2.1.78.1.36", "1.3.6.1.4.1.2011.5.2.1.78.1.37"},
		{"1.3.6.1.4.1.2011.5.2.1.15.1.70", "1.3.6.1.4.1.2011.5.2.1.15.1.71"},
	}
	for _, fb := range fallbacks {
		if fb.up == up && fb.dn == dn {
			continue
		}
		out = append(out, fb)
	}
	return out
}

func readFlow64Counters(ctx context.Context, host, community string, profile Profile, idx string, timeout time.Duration) (up, dn uint64, err error) {
	suffix := "." + idx
	for _, pair := range flow64OIDCandidates(profile) {
		upOID := probing.NormalizeSNMPOID(pair.up + suffix)
		dnOID := probing.NormalizeSNMPOID(pair.dn + suffix)
		vars, walkErr := probing.SNMPGetMany(ctx, host, community, "2c", timeout, 1, []string{upOID, dnOID}, 2)
		if walkErr != "" && len(vars) == 0 {
			continue
		}
		var gotUp, gotDn bool
		for _, v := range vars {
			oid := probing.NormalizeSNMPOID(v.OID)
			n, ok := parseUint64Metric(v.Value)
			if !ok {
				continue
			}
			switch oid {
			case upOID:
				up, gotUp = n, true
			case dnOID:
				dn, gotDn = n, true
			}
		}
		if gotUp || gotDn {
			return up, dn, nil
		}
	}
	return 0, 0, fmt.Errorf("contadores Flow64 não encontrados para índice %s", idx)
}

func flow64Delta(prev, next uint64) uint64 {
	if next >= prev {
		return next - prev
	}
	return next
}

func parseUint64Metric(v string) (uint64, bool) {
	v = strings.TrimSpace(v)
	if v == "" || !probing.SNMPValueUsable(v) {
		return 0, false
	}
	if n, ok := parseInt64Metric(v); ok && n >= 0 {
		return uint64(n), true
	}
	return 0, false
}

// FormatBitrateBps formata taxa em bps (preferência Mbps).
func FormatBitrateBps(bps int64) string {
	if bps < 0 {
		bps = 0
	}
	if bps == 0 {
		return "0 bps"
	}
	mbps := float64(bps) / 1_000_000
	if mbps >= 100 {
		return fmt.Sprintf("%.0f Mbps", mbps)
	}
	if mbps >= 10 {
		return fmt.Sprintf("%.1f Mbps", mbps)
	}
	if mbps >= 1 {
		return fmt.Sprintf("%.2f Mbps", mbps)
	}
	kbps := float64(bps) / 1000
	if kbps >= 1 {
		return fmt.Sprintf("%.1f Kbps", kbps)
	}
	return fmt.Sprintf("%d bps", bps)
}
