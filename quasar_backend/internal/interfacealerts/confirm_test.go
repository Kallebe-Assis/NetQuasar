package interfacealerts

import (
	"testing"

	"github.com/netquasar/netquasar/quasar_backend/internal/snmpifparse"
)

func TestShouldConfirmIfaceDownAfterStreak(t *testing.T) {
	t.Parallel()
	up := snmpifparse.IfRow{IfIndex: 1, OperStatus: 1}
	down := snmpifparse.IfRow{IfIndex: 1, OperStatus: 2}
	missing := snmpifparse.IfRow{IfIndex: 1, OperStatus: 0}

	if shouldConfirmIfaceDownAfterStreak(up, down, down, true) != true {
		t.Fatal("older UP + 2× DOWN must confirm")
	}
	if shouldConfirmIfaceDownAfterStreak(up, up, down, true) {
		t.Fatal("only one DOWN must not confirm")
	}
	if shouldConfirmIfaceDownAfterStreak(up, down, up, true) {
		t.Fatal("recovered UP must not confirm")
	}
	if shouldConfirmIfaceDownAfterStreak(up, down, missing, true) {
		t.Fatal("missing oper must not confirm")
	}
	if shouldConfirmIfaceDownAfterStreak(snmpifparse.IfRow{}, up, down, false) {
		t.Fatal("without older snapshot must wait next cycle")
	}
}

func TestSuspectMassInterfaceDrop(t *testing.T) {
	t.Parallel()
	prev := map[int]snmpifparse.IfRow{}
	curr := map[int]snmpifparse.IfRow{}
	for i := 1; i <= 10; i++ {
		prev[i] = snmpifparse.IfRow{IfIndex: i, OperStatus: 1}
		// 6 of 10 lost oper status (partial walk) → mass drop
		if i <= 6 {
			curr[i] = snmpifparse.IfRow{IfIndex: i, OperStatus: 0}
		} else {
			curr[i] = snmpifparse.IfRow{IfIndex: i, OperStatus: 1}
		}
	}
	if !suspectMassInterfaceDrop(prev, curr) {
		t.Fatal("expected mass drop on partial walk")
	}
	for i := 1; i <= 10; i++ {
		curr[i] = snmpifparse.IfRow{IfIndex: i, OperStatus: 1}
	}
	curr[1] = snmpifparse.IfRow{IfIndex: 1, OperStatus: 2}
	if suspectMassInterfaceDrop(prev, curr) {
		t.Fatal("single real down must not look like mass drop")
	}
}

func TestIfaceIsDownKnownIgnoresMissing(t *testing.T) {
	t.Parallel()
	if ifaceIsDownKnown(snmpifparse.IfRow{OperStatus: 0}) {
		t.Fatal("oper 0 must not count as down")
	}
	if !ifaceIsDownKnown(snmpifparse.IfRow{OperStatus: 2}) {
		t.Fatal("oper 2 must count as down")
	}
	if !ifaceIsUp(snmpifparse.IfRow{OperStatus: 1}) {
		t.Fatal("oper 1 must be up")
	}
}
