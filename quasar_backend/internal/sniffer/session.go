package sniffer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Tectos de segurança por sessão — full-payload em memória pode crescer rápido; isto evita
// que uma captura esquecida a correr esgote a RAM do contentor. O utilizador pode sempre
// parar/guardar antes disso; ao atingir o tecto a captura pára sozinha (fica "stopped").
const (
	maxPacketsPerSession = 50_000
	maxBytesPerSession   = 256 << 20 // 256MB de payload bruto
	maxSessionDuration   = 30 * time.Minute
)

type Source string

const (
	SourceLocal  Source = "local"
	SourceDevice Source = "device"
)

type SessionStatus string

const (
	StatusRunning SessionStatus = "running"
	StatusStopped SessionStatus = "stopped"
	StatusSaved   SessionStatus = "saved"
)

// Session é uma captura ao vivo (ou já parada, ainda não guardada/descartada) — os
// pacotes ficam em memória até o utilizador decidir "Guardar" (grava .pcap + liberta a
// memória) ou "Descartar" (só liberta a memória).
type Session struct {
	ID        uuid.UUID
	Name      string
	Source    Source
	DeviceID  *uuid.UUID
	Interface string
	StartedAt time.Time
	StoppedAt *time.Time
	Status    SessionStatus
	Err       string

	mu         sync.Mutex
	packets    []Packet
	totalBytes int64
	cancel     context.CancelFunc
}

func (s *Session) addPacket(p Packet) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.packets) >= maxPacketsPerSession || s.totalBytes >= maxBytesPerSession {
		return false
	}
	p.Seq = len(s.packets) + 1
	s.packets = append(s.packets, p)
	s.totalBytes += int64(len(p.Raw))
	return true
}

// SnapshotSince devolve os pacotes com Seq > seq (poll incremental do UI ao vivo).
func (s *Session) SnapshotSince(seq int) []Packet {
	s.mu.Lock()
	defer s.mu.Unlock()
	if seq >= len(s.packets) {
		return nil
	}
	out := make([]Packet, len(s.packets)-seq)
	copy(out, s.packets[seq:])
	return out
}

// All devolve todos os pacotes capturados até agora (usado para o detalhe de um pacote
// e para gravar em .pcap ao guardar a sessão).
func (s *Session) All() []Packet {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Packet, len(s.packets))
	copy(out, s.packets)
	return out
}

func (s *Session) Count() (int, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.packets), s.totalBytes
}

func (s *Session) setStatus(st SessionStatus) {
	s.mu.Lock()
	s.Status = st
	s.mu.Unlock()
}

// Manager mantém as sessões de captura activas/pendentes de decisão nesta instância do
// servidor (não persistidas — só o que for "guardado" vira uma linha em network_captures).
type Manager struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*Session
	dataDir  string
}

func NewManager(dataDir string) *Manager {
	return &Manager{sessions: map[uuid.UUID]*Session{}, dataDir: dataDir}
}

type StartLocalParams struct {
	Name      string
	Interface string
}

func (m *Manager) StartLocal(p StartLocalParams) (*Session, error) {
	sess := &Session{
		ID: uuid.New(), Name: p.Name, Source: SourceLocal, Interface: p.Interface,
		StartedAt: time.Now(), Status: StatusRunning,
	}
	ctx, cancel := context.WithTimeout(context.Background(), maxSessionDuration)
	sess.cancel = cancel
	m.mu.Lock()
	m.sessions[sess.ID] = sess
	m.mu.Unlock()

	go func() {
		err := captureLocal(ctx, p.Interface, func(raw []byte, ts time.Time) bool {
			pk := DecodePacket(raw)
			pk.Ts = ts
			pk.Raw = raw
			return sess.addPacket(pk)
		})
		m.finish(sess, err)
	}()
	return sess, nil
}

type StartDeviceParams struct {
	Name      string
	DeviceID  uuid.UUID
	Host      string
	Port      string
	User      string
	Password  string
	Interface string
}

func (m *Manager) StartDevice(p StartDeviceParams) (*Session, error) {
	deviceID := p.DeviceID
	sess := &Session{
		ID: uuid.New(), Name: p.Name, Source: SourceDevice, DeviceID: &deviceID, Interface: p.Interface,
		StartedAt: time.Now(), Status: StatusRunning,
	}
	ctx, cancel := context.WithTimeout(context.Background(), maxSessionDuration)
	sess.cancel = cancel
	m.mu.Lock()
	m.sessions[sess.ID] = sess
	m.mu.Unlock()

	go func() {
		err := captureRemoteSSH(ctx, remoteSSHParams{
			Host: p.Host, Port: p.Port, User: p.User, Password: p.Password, Interface: p.Interface,
		}, func(raw []byte, ts time.Time) bool {
			pk := DecodePacket(raw)
			pk.Ts = ts
			pk.Raw = raw
			return sess.addPacket(pk)
		})
		m.finish(sess, err)
	}()
	return sess, nil
}

func (m *Manager) finish(sess *Session, err error) {
	sess.mu.Lock()
	now := time.Now()
	sess.StoppedAt = &now
	if sess.Status == StatusRunning {
		sess.Status = StatusStopped
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		sess.Err = err.Error()
	}
	sess.mu.Unlock()
}

func (m *Manager) Get(id uuid.UUID) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) Stop(id uuid.UUID) error {
	s, ok := m.Get(id)
	if !ok {
		return errors.New("sessão não encontrada")
	}
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// Save grava a sessão em .pcap (nome = ID da sessão) e liberta a memória — a partir daqui
// os pacotes lêem-se do ficheiro. O chamador HTTP é responsável por gravar os metadados
// (nome, descrição) na tabela network_captures usando o caminho devolvido.
func (m *Manager) Save(id uuid.UUID) (path string, count int, totalBytes int64, err error) {
	s, ok := m.Get(id)
	if !ok {
		return "", 0, 0, errors.New("sessão não encontrada")
	}
	pkts := s.All()
	if err := os.MkdirAll(m.dataDir, 0o755); err != nil {
		return "", 0, 0, err
	}
	path = filepath.Join(m.dataDir, id.String()+".pcap")
	if err := WritePcapFile(path, pkts); err != nil {
		return "", 0, 0, err
	}
	count, totalBytes = s.Count()
	s.setStatus(StatusSaved)
	m.Discard(id)
	return path, count, totalBytes, nil
}

// Discard cancela a captura (se ainda a correr) e liberta a sessão da memória, sem gravar nada.
func (m *Manager) Discard(id uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		delete(m.sessions, id)
	}
}
