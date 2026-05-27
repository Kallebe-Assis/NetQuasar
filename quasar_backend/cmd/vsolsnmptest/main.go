// Teste pontual SNMP OLT VSOL — IF-MIB (ONU por PON) + enterprise 13464 / 37950.
//
//	go run ./cmd/vsolsnmptest -host 10.255.150.2 -community VERIFICA
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/oltifderive"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

func main() {
	host := flag.String("host", "10.255.150.2", "IP OLT")
	community := flag.String("community", "VERIFICA", "community v2c")
	timeout := flag.Duration("timeout", 60*time.Second, "timeout por walk")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	fmt.Printf("=== SNMP probe %s community=%s ===\n\n", *host, *community)

	// IF-MIB
	fmt.Println("--- IF-MIB (ifDescr / contagem ONU por PON) ---")
	w1, tr1, e1 := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: *host, Community: *community, RootOID: "1.3.6.1.2.1.2.2.1.2",
		Version: "2c", Timeout: *timeout, MaxRows: 15000,
	})
	w2, tr2, e2 := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: *host, Community: *community, RootOID: "1.3.6.1.2.1.31.1.1.1.1",
		Version: "2c", Timeout: *timeout, MaxRows: 15000,
	})
	wIf := append(w1, w2...)
	fmt.Printf("ifDescr rows=%d ifName rows=%d truncated=%v err=%v %v\n", len(w1), len(w2), tr1||tr2, e1, e2)

	// Build minimal vars for BuildIfTable — need full 2.2.1 for operStatus; use combined walk
	wFull, trF, eF := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: *host, Community: *community, RootOID: "1.3.6.1.2.1.2.2.1",
		Version: "2c", Timeout: *timeout, Retries: 1, MaxRows: 20000,
	})
	wX, trX, eX := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: *host, Community: *community, RootOID: "1.3.6.1.2.1.31.1.1.1",
		Version: "2c", Timeout: *timeout, MaxRows: 20000,
	})
	allIf := append(wFull, wX...)
	fmt.Printf("IF full walk rows=%d truncated=%v err=%v %v\n", len(allIf), trF||trX, eF, eX)

	ifRows := snmpifparse.BuildIfTable(allIf)
	byPon := map[string]int{}
	byPonUp := map[string]int{}
	var onuSamples []string
	for _, r := range ifRows {
		disp := strings.TrimSpace(r.IfName)
		if disp == "" {
			disp = strings.TrimSpace(r.Descr)
		}
		c, _, ok := oltifderive.PonCompactFromOnuIface(disp, r.Descr)
		if !ok || c == "" {
			continue
		}
		byPon[c]++
		if r.OperStatus == 1 {
			byPonUp[c]++
		}
		if len(onuSamples) < 5 {
			onuSamples = append(onuSamples, fmt.Sprintf("if%d %s oper=%d", r.IfIndex, disp, r.OperStatus))
		}
	}
	keys := make([]string, 0, len(byPon))
	for k := range byPon {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("PONs com ONU (IF-MIB): %d\n", len(keys))
	for _, k := range keys {
		fmt.Printf("  PON compact=%s total=%d up(oper=1)=%d down=%d\n", k, byPon[k], byPonUp[k], byPon[k]-byPonUp[k])
	}
	fmt.Println("Amostras ONU:", strings.Join(onuSamples, " | "))

	ponPhy := oltifderive.PonPortsFromSNMPVars(allIf)
	fmt.Printf("Portas PON físicas (GPON0/N): %d\n", len(ponPhy))
	for _, p := range ponPhy {
		fmt.Printf("  %s %v total_if=%v\n", p["name"], p["id"], p["onu_total"])
	}

	// Enterprise 13464 (reportado pelo utilizador)
	fmt.Println("\n--- Enterprise 1.3.6.1.4.1.13464.1.11.3.1 ---")
	w13464, tr134, e134 := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: *host, Community: *community, RootOID: "1.3.6.1.4.1.13464.1.11.3.1",
		Version: "2c", Timeout: *timeout, Retries: 1, MaxRows: 5000,
	})
	fmt.Printf("rows=%d truncated=%v err=%q\n", len(w13464), tr134, e134)
	for i, v := range w13464 {
		if i >= 12 {
			fmt.Printf("  ... +%d linhas\n", len(w13464)-12)
			break
		}
		val := v.Value
		if len(val) > 60 {
			val = val[:57] + "..."
		}
		fmt.Printf("  %s = %s\n", v.OID, val)
	}

	// VSOL clássico 37950
	fmt.Println("\n--- VSOL MIB 37950 gOnuAuthList (status) ---")
	w379, tr379, e379 := probing.SNMPWalk(ctx, probing.SNMPWalkParams{
		Host: *host, Community: *community, RootOID: vsolparse.OIDGOnuAuthList + ".1",
		Version: "2c", Timeout: *timeout, Retries: 1, MaxRows: 15000,
	})
	fmt.Printf("sta rows=%d truncated=%v err=%q\n", len(w379), tr379, e379)

	refs := vsolparse.OnuRefsFromIfRows(ifRows)
	fmt.Printf("ONU refs para GET: %d\n", len(refs))
	mibTO := vsolparse.CollectTimeout(len(refs), true)
	mibCtx, mibCancel := context.WithTimeout(ctx, mibTO)
	defer mibCancel()
	coll := vsolparse.CollectOLT(mibCtx, *host, *community, refs, true)
	sum, pons, onuRows := vsolparse.FromSNMPWalk(coll.Vars, false)
	fmt.Printf("vsolparse CollectOLT: passos=%d vars=%d onus=%d online=%v offline=%v note=%s\n",
		len(coll.Steps), len(coll.Vars), sum["vsol_onu_count"], sum["vsol_onu_online"], sum["vsol_onu_offline"], coll.Note)
	for _, p := range pons {
		fmt.Printf("  PON %v id=%v tot=%v on=%v off=%v\n", p["name"], p["id"], p["onu_total"], p["onu_online"], p["onu_offline"])
	}
	if len(onuRows) > 0 {
		r0 := onuRows[0]
		fmt.Printf("  amostra ONU: pon=%v onu=%v phase=%v omcc=%v online=%v\n", r0["pon"], r0["onu"], r0["phase_sta"], r0["omcc_sta"], r0["online"])
	}

	_ = wIf
	if len(keys) == 0 && len(w13464) == 0 && len(pons) == 0 {
		os.Exit(1)
	}
}
