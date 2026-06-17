package alertstore

import "fmt"

// MatchKind define como identificar um alerta aberto (deduplicação).
type MatchKind int

const (
	// MatchDeviceOnly — um alerta aberto por (device_id, alert_type).
	MatchDeviceOnly MatchKind = iota
	// MatchMetaKey — deduplica por meta.key.
	MatchMetaKey
	// MatchIfIndex — deduplica por meta.if_index (SFP MikroTik).
	MatchIfIndex
)

// Match critério de unicidade para alertas abertos.
type Match struct {
	Kind    MatchKind
	MetaKey string
	IfIndex int
}

func (m Match) notExistsClause(keyParam int) string {
	switch m.Kind {
	case MatchMetaKey:
		return fmt.Sprintf(" AND (ai.meta->>'key') = $%d", keyParam)
	case MatchIfIndex:
		return fmt.Sprintf(" AND (ai.meta->>'if_index')::int = $%d", keyParam)
	default:
		return ""
	}
}

func (m Match) whereClause(keyParam int) string {
	switch m.Kind {
	case MatchMetaKey:
		return fmt.Sprintf(" AND (meta->>'key') = $%d", keyParam)
	case MatchIfIndex:
		return fmt.Sprintf(" AND (meta->>'if_index')::int = $%d", keyParam)
	default:
		return ""
	}
}

func (m Match) appendKeyArg(args []any) []any {
	switch m.Kind {
	case MatchMetaKey:
		return append(args, m.MetaKey)
	case MatchIfIndex:
		return append(args, m.IfIndex)
	default:
		return args
	}
}
