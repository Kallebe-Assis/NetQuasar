// monitorprobe executa SNMP (IF-MIB + IF-MIB-X) como o worker de PON e imprime tempos na consola.
// Opcional: teste Telnet TCP (banner/login). Sem base de dados.
//
// Exemplo:
//
//	go run ./cmd/monitorprobe -host 10.20.30.40 -community minha_ro
//	go run ./cmd/monitorprobe -host 10.20.30.40 -community x -telnet-probe -telnet-user admin -telnet-pass secret
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpmikrotik"
	"github.com/rs/zerolog"
)

func main() {
	host := flag.String("host", "", "IP ou hostname da OLT (SNMP + telnet)")
	community := flag.String("community", "public", "SNMP v2 community")
	timeout := flag.Duration("snmp-timeout", 38*time.Second, "timeout por pdu SNMP")
	retries := flag.Int("snmp-retries", 1, "retries gosnmp (0–3)")
	pass2 := flag.Bool("second-pass", false, "repetir walk com timeout 58s + 2 retries (simula retentativa do worker)")

	telProbe := flag.Bool("telnet-probe", false, "após SNMP, testar TCP 23 / login texto")
	tPort := flag.String("telnet-port", "23", "porta telnet")
	tUser := flag.String("telnet-user", "", "utilizador (opcional)")
	tPass := flag.String("telnet-pass", "", "palavra-passe (opcional)")
	flag.Parse()

	out := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	log := zerolog.New(out).With().Timestamp().Logger()

	if strings.TrimSpace(*host) == "" {
		log.Error().Msg("obrigatório: -host")
		flag.Usage()
		os.Exit(2)
	}
	if *retries < 0 || *retries > 3 {
		log.Fatal().Msg("snmp-retries entre 0 e 3")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	t0 := time.Now()
	log.Info().Str("stage", "start").Str("target", *host).Msg("Monitor probe")

	type step struct {
		name string
		oid  string
		max  int
	}
	steps := []step{
		{"IF-MIB ifTable subtree", "1.3.6.1.2.1.2.2.1", 14000},
		{"IF-MIB ifXTable subtree", "1.3.6.1.2.1.31.1.1.1", 20000},
	}

	appendWalk := func(label string, d time.Duration, r int) []probing.SNMPVar {
		var vars []probing.SNMPVar
		passStart := time.Now()
		for _, st := range steps {
			sw := time.Now()
			row, truncated, note := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
				Host: *host, Port: 161, Community: *community, RootOID: st.oid,
				Version: "2c", Timeout: d, Retries: r, MaxRows: st.max,
			})
			log.Info().
				Str("pass", label).
				Str("snmpSubtree", st.name).
				Int("rows", len(row)).
				Bool("truncated", truncated).
				Str("walkErr", note).
				Int64("duration_ms", time.Since(sw).Milliseconds()).
				Msg("SNMP bulkwalk concluído")
			vars = append(vars, row...)
		}
		log.Info().Str("pass", label).Int64("pass_total_ms", time.Since(passStart).Milliseconds()).Msg("pass SNMP completo")
		return vars
	}

	var all []probing.SNMPVar
	all = appendWalk("1", *timeout, *retries)
	if *pass2 {
		time.Sleep(4 * time.Second)
		all = appendWalk("2_slow", 58*time.Second, 2)
	}

	td := time.Now()
	ifRows := snmpifparse.BuildIfTable(all)
	optMap := snmpmikrotik.OpticalPowerByIfIndex(ifRows, all)
	pons, sumPatch := oltifderive.DeriveFromIfRows(ifRows, optMap)
	sumOn := sumOnuOnlineInPonRows(pons)

	log.Info().
		Int64("derive_ms", time.Since(td).Milliseconds()).
		Int("interfaces_parsed", len(ifRows)).
		Int("pon_rows", len(pons)).
		Float64("derived_onu_online_sum", sumOn).
		Msg("Derive IF-MIB → ONUs por PON")
	if sumPatch != nil {
		fmt.Fprintf(os.Stdout, "Summary patch: %+v\n", sumPatch)
	}

	if *telProbe {
		tt := time.Now()
		res := probing.TelnetProbe(ctx, probing.TelnetTestParams{
			Host: *host, Port: *tPort, Timeout: 12 * time.Second,
			User: *tUser, Password: *tPass, MaxReadBytes: 4096,
		})
		log.Info().
			Bool("ok", res.OK).
			Int64("latency_ms", res.LatencyMs).
			Str("error", res.Error).
			Int64("probe_total_ms", time.Since(tt).Milliseconds()).
			Msg("Telnet TCP/login (diag)")
		b := res.Banner
		if len(b) > 200 {
			b = b[:200] + "…"
		}
		if b != "" {
			fmt.Fprintf(os.Stdout, "Banner/snippet:\n%s\n", b)
		}
	}

	log.Info().
		Int64("wall_clock_ms_total", time.Since(t0).Milliseconds()).
		Msg("Monitor probe terminado — compare if_mib_ms vs ifx_ms para localizar gargalos")
}

func sumOnuOnlineInPonRows(pons []map[string]any) float64 {
	var s float64
	for _, p := range pons {
		if n, ok := oltifderive.OnuOnlineFromRow(p); ok {
			s += n
		}
	}
	return s
}
