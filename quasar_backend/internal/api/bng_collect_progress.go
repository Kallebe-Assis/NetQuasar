package api

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type bngCollectJob struct {
	Status           string     `json:"status"`
	Phase            string     `json:"phase,omitempty"`
	LoginsLoaded     int        `json:"logins_loaded"`
	SessionsEnriched int        `json:"sessions_enriched"`
	SessionsTotal    int        `json:"sessions_total"`
	SessionCount     int        `json:"session_count"`
	Message          string     `json:"message,omitempty"`
	Error            string     `json:"error,omitempty"`
	Done             bool       `json:"done"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

type bngCollectProgressStore struct {
	mu   sync.RWMutex
	jobs map[uuid.UUID]*bngCollectJob
}

func newBngCollectProgressStore() *bngCollectProgressStore {
	return &bngCollectProgressStore{jobs: make(map[uuid.UUID]*bngCollectJob)}
}

func (s *bngCollectProgressStore) start(deviceID uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[deviceID]; ok && j != nil && !j.Done && j.Status == "running" {
		return false
	}
	s.jobs[deviceID] = &bngCollectJob{
		Status:    "running",
		Phase:     "login",
		Message:   "A iniciar consulta SNMP…",
		StartedAt: time.Now(),
	}
	return true
}

func (s *bngCollectProgressStore) update(deviceID uuid.UUID, fn func(*bngCollectJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[deviceID]
	if !ok || j == nil {
		return
	}
	fn(j)
}

func (s *bngCollectProgressStore) finish(deviceID uuid.UUID, sessionCount int, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[deviceID]
	if !ok || j == nil {
		return
	}
	now := time.Now()
	j.FinishedAt = &now
	j.Done = true
	j.SessionCount = sessionCount
	if errMsg != "" {
		j.Status = "error"
		j.Error = errMsg
		j.Message = "Consulta falhou."
		return
	}
	j.Status = "done"
	j.Message = "Consulta concluída."
}

func (s *bngCollectProgressStore) get(deviceID uuid.UUID) bngCollectJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[deviceID]
	if !ok || j == nil {
		return bngCollectJob{Status: "idle", Done: true}
	}
	cp := *j
	return cp
}
