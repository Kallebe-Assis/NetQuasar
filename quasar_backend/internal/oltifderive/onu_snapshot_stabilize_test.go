package oltifderive

import (
	"encoding/json"
	"testing"
)

func TestStablePonRowKey_nameFallback(t *testing.T) {
	row := map[string]any{"name": "GPON0/3", "id": ""}
	if got := StablePonRowKey(row); got == "" {
		t.Fatalf("expected non-empty key from name, got %q", got)
	}
}

func TestStabilizePonSnapshotRows_firstZeroHoldsHighPrev(t *testing.T) {
	prev := []map[string]any{{"id": "01", "onu_online": 40.0}}
	cur := []map[string]any{{"id": "01", "onu_online": 0.0}}
	out, patch := StabilizePonSnapshotRows(prev, cur, nil)
	if len(out) != 1 {
		t.Fatalf("len(out)=%d", len(out))
	}
	on, ok := OnuOnlineFromRow(out[0])
	if !ok || on != 40 {
		t.Fatalf("expected held 40, got ok=%v on=%v", ok, on)
	}
	if out[0]["onu_online_snap_held"] != true {
		t.Fatalf("expected snap_held flag")
	}
	streak, _ := patch[summaryKeyOnuZeroConfirm].(map[string]any)
	if streak == nil || streak["01"] != float64(1) && streak["01"] != 1 {
		t.Fatalf("expected streak 01=1, patch=%v", patch)
	}
}

func TestStabilizePonSnapshotRows_secondZeroStillHoldsUntilThird(t *testing.T) {
	prev := []map[string]any{{"id": "01", "onu_online": 40.0}}
	cur := []map[string]any{{"id": "01", "onu_online": 0.0}}
	prevSumm := map[string]any{
		summaryKeyOnuZeroConfirm: map[string]any{"01": 1},
	}
	out, _ := StabilizePonSnapshotRows(prev, cur, prevSumm)
	on, ok := OnuOnlineFromRow(out[0])
	if !ok || on != 40 {
		t.Fatalf("expected still held at 40 on 2nd streak, got ok=%v on=%v", ok, on)
	}
}

func TestStabilizePonSnapshotRows_thirdZeroAccepts(t *testing.T) {
	prev := []map[string]any{{"id": "01", "onu_online": 40.0}}
	cur := []map[string]any{{"id": "01", "onu_online": 0.0}}
	prevSumm := map[string]any{
		summaryKeyOnuZeroConfirm: map[string]any{"01": 2},
	}
	out, _ := StabilizePonSnapshotRows(prev, cur, prevSumm)
	on, ok := OnuOnlineFromRow(out[0])
	if !ok || on != 0 {
		t.Fatalf("expected accepted 0 after 3ª leitura suspeita, got ok=%v on=%v", ok, on)
	}
	if out[0]["onu_online_snap_held"] != nil {
		t.Fatalf("should not keep held on third confirm")
	}
}

func TestStabilizePonSnapshotRows_smallPonNotHeld(t *testing.T) {
	prev := []map[string]any{{"id": "01", "onu_online": 3.0}}
	cur := []map[string]any{{"id": "01", "onu_online": 0.0}}
	out, _ := StabilizePonSnapshotRows(prev, cur, nil)
	on, _ := OnuOnlineFromRow(out[0])
	if on != 0 {
		t.Fatalf("abaixo do mínimo de dúvida não deve segurar valor anterior, got on=%v", on)
	}
}

func TestOnuOnlineFromRow_jsonNumber(t *testing.T) {
	var m map[string]any
	_ = json.Unmarshal([]byte(`{"onu_online":17}`), &m)
	n, ok := OnuOnlineFromRow(m)
	if !ok || n != 17 {
		t.Fatalf("json float decode: ok=%v n=%v", ok, n)
	}
}
