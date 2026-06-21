package monitorworker

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type oltCollectCandidate struct {
	row    pingableDeviceRow
	lastAt time.Time
}

// scheduleOltCollectCandidates devolve todas as OLTs elegíveis para coleta neste ciclo.
func scheduleOltCollectCandidates(
	devices []pingableDeviceRow,
	lastByDevice map[uuid.UUID]time.Time,
	opts SweepOpts,
	interval time.Duration,
	defCommunity *string,
) []oltCollectCandidate {
	var due []oltCollectCandidate
	for _, row := range devices {
		comm := resolveSNMPCommunity(row, defCommunity)
		if comm == "" {
			continue
		}
		lastAt := lastByDevice[row.id]
		if !sweepShouldCollectDevice(opts, lastAt, interval) {
			continue
		}
		due = append(due, oltCollectCandidate{row: row, lastAt: lastAt})
	}
	if len(due) == 0 {
		return nil
	}
	sort.Slice(due, func(i, j int) bool {
		li, lj := due[i].lastAt, due[j].lastAt
		if li.IsZero() != lj.IsZero() {
			return li.IsZero()
		}
		if !li.Equal(lj) {
			return li.Before(lj)
		}
		return strings.TrimSpace(due[i].row.description) < strings.TrimSpace(due[j].row.description)
	})
	return due
}

// oltCollectCandidatesFromDevices constrói candidatos a partir da lista já filtrada (pipeline).
func oltCollectCandidatesFromDevices(devices []pingableDeviceRow, defCommunity *string) []oltCollectCandidate {
	out := make([]oltCollectCandidate, 0, len(devices))
	for _, row := range devices {
		if resolveSNMPCommunity(row, defCommunity) == "" {
			continue
		}
		out = append(out, oltCollectCandidate{row: row})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.TrimSpace(out[i].row.description) < strings.TrimSpace(out[j].row.description)
	})
	return out
}
