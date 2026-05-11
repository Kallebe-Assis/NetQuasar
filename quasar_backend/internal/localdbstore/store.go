// Package localdbstore persiste credenciais PostgreSQL no disco (primeira configuração / portabilidade entre reinícios).
package localdbstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/config"
)

const fileName = "database-credentials.json"

// DataDir diretório base (NETQUASAR_DATA_DIR ou "data" relativo ao cwd).
func DataDir() string {
	if d := strings.TrimSpace(os.Getenv("NETQUASAR_DATA_DIR")); d != "" {
		return d
	}
	return "data"
}

func filePath() string {
	return filepath.Join(DataDir(), fileName)
}

// Credentials formato JSON no disco (password em claro — ficheiro com permissões restritas).
type Credentials struct {
	DatabaseURL string `json:"database_url,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        int    `json:"port,omitempty"`
	DBUser      string `json:"db_user,omitempty"`
	DBPassword  string `json:"db_password,omitempty"`
	DBName      string `json:"db_name,omitempty"`
	SSLMode     string `json:"ssl_mode,omitempty"`
}

// Read lê o ficheiro local; devolve (nil, nil) se não existir.
func Read() (*Credentials, error) {
	p := filePath()
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &c, nil
}

// ApplyToConfig copia credenciais para cfg (não altera campos já preenchidos).
func ApplyToConfig(cfg *config.Config, c *Credentials) {
	if c == nil {
		return
	}
	if strings.TrimSpace(c.DatabaseURL) != "" {
		cfg.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
		return
	}
	if strings.TrimSpace(c.Host) != "" {
		cfg.DBHost = strings.TrimSpace(c.Host)
	}
	if c.Port > 0 {
		cfg.DBPort = c.Port
	}
	if strings.TrimSpace(c.DBUser) != "" {
		cfg.DBUser = strings.TrimSpace(c.DBUser)
	}
	if c.DBPassword != "" {
		cfg.DBPassword = c.DBPassword
	}
	if strings.TrimSpace(c.DBName) != "" {
		cfg.DBName = strings.TrimSpace(c.DBName)
	}
	if strings.TrimSpace(c.SSLMode) != "" {
		cfg.DBSSLMode = strings.TrimSpace(c.SSLMode)
	}
}

// MergeIntoConfig aplica o ficheiro local só quando o ambiente ainda não define ligação completa (URL ou password).
func MergeIntoConfig(cfg *config.Config) error {
	if cfg.CanConnectDatabase() {
		return nil
	}
	creds, err := Read()
	if err != nil {
		return err
	}
	if creds == nil {
		return nil
	}
	ApplyToConfig(cfg, creds)
	return nil
}

// Write grava atomicamente com permissão 0600.
func Write(c *Credentials) error {
	if c == nil {
		return errors.New("localdbstore: credenciais vazias")
	}
	dir := DataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := filePath()
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// WriteFromDSN persiste uma URL completa (preferido após apply pela UI).
func WriteFromDSN(dsn string) error {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return errors.New("localdbstore: DSN vazio")
	}
	return Write(&Credentials{DatabaseURL: dsn})
}
