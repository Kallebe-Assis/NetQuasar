package alertstore

import "testing"

func TestMatchNotExistsClause(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind MatchKind
		want string
	}{
		{MatchDeviceOnly, ""},
		{MatchMetaKey, " AND (ai.meta->>'key') = $3"},
		{MatchIfIndex, " AND (ai.meta->>'if_index')::int = $5"},
	}
	for _, c := range cases {
		m := Match{Kind: c.kind, MetaKey: "k", IfIndex: 7}
		got := m.notExistsClause(len("placeholder") + 1) // param index varies; check suffix pattern
		if c.kind == MatchMetaKey {
			m2 := Match{Kind: MatchMetaKey}
			if m2.notExistsClause(3) != c.want {
				t.Fatalf("MetaKey: got %q want %q", m2.notExistsClause(3), c.want)
			}
		}
		if c.kind == MatchIfIndex {
			m2 := Match{Kind: MatchIfIndex}
			if m2.notExistsClause(5) != c.want {
				t.Fatalf("IfIndex: got %q want %q", m2.notExistsClause(5), c.want)
			}
		}
		if c.kind == MatchDeviceOnly && got != "" {
			t.Fatalf("DeviceOnly should be empty, got %q", got)
		}
		_ = m
	}
}

func TestMatchAppendKeyArg(t *testing.T) {
	t.Parallel()
	args := []any{"a", "b"}
	out := Match{Kind: MatchMetaKey, MetaKey: "telemetry:cpu"}.appendKeyArg(args)
	if len(out) != 3 || out[2] != "telemetry:cpu" {
		t.Fatalf("meta key append: %+v", out)
	}
	out2 := Match{Kind: MatchIfIndex, IfIndex: 42}.appendKeyArg(args)
	if len(out2) != 3 || out2[2] != 42 {
		t.Fatalf("if index append: %+v", out2)
	}
}
