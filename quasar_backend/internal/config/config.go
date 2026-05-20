// Package config carrega variáveis de ambiente para deploy multi-empresa (BD separado por instância).
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config agrega parâmetros de runtime. Nenhuma credencial é hardcoded.
type Config struct {
	HTTPAddr         string
	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	ShutdownTimeout  time.Duration
	LogLevel         string
	CORSOrigins      []string

	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	// DBSSLRootCert — caminho absoluto ou relativo a um PEM (ex.: CA raiz Supabase). Variável NETQUASAR_DB_SSLROOTCERT.
	DBSSLRootCert string
	// DatabaseURL, se definido, tem precedência sobre os campos DB* individuais.
	DatabaseURL string

	APIKeys []string // NETQUASAR_API_KEYS=key1,key2 (opcional; se vazio, auth desativada em dev — ver README)

	// SessionSecret — NETQUASAR_SESSION_SECRET; assinatura HS256 dos JWT de sessão (login na UI). Se vazio com API keys, usa a primeira key como segredo (legado).
	SessionSecret string

	// EmbeddedUI — servir frontend estático compilado incluído no binário (NetQuasar em único processo).
	EmbeddedUI bool
}

// RequireAuth indica se requisições /api/* exigem X-API-Key ou JWT de utilizador (ambiente de produção típico).
func (c *Config) RequireAuth() bool {
	return len(c.APIKeys) > 0 || strings.TrimSpace(c.SessionSecret) != ""
}

// JWTSigningSecret segredo HS256 para emitir/validar JWT de login.
func (c *Config) JWTSigningSecret() []byte {
	if s := strings.TrimSpace(c.SessionSecret); s != "" {
		return []byte(s)
	}
	if len(c.APIKeys) > 0 {
		return []byte(strings.TrimSpace(c.APIKeys[0]))
	}
	return []byte("netquasar-dev-insecure-jwt-do-not-use-in-prod")
}

// CanConnectDatabase indica se há DSN ou palavra-passe suficiente para abrir o Postgres (não inclui ficheiro local).
func (c *Config) CanConnectDatabase() bool {
	if strings.TrimSpace(c.DatabaseURL) != "" {
		return true
	}
	return strings.TrimSpace(c.DBPassword) != ""
}

func getenv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func mustAtoi(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func parseBoolEnv(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func durationEnv(key string, def time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

// Load lê o ambiente. Falha se configuração mínima de banco estiver inconsistente.
func Load() (*Config, error) {
	c := &Config{
		HTTPAddr:         getenv("NETQUASAR_HTTP_ADDR", ":8080"),
		HTTPReadTimeout:  durationEnv("NETQUASAR_HTTP_READ_TIMEOUT", 15*time.Second),
		HTTPWriteTimeout: durationEnv("NETQUASAR_HTTP_WRITE_TIMEOUT", 120*time.Second),
		ShutdownTimeout:  durationEnv("NETQUASAR_SHUTDOWN_TIMEOUT", 20*time.Second),
		LogLevel:         getenv("NETQUASAR_LOG_LEVEL", "info"),
		DBHost:           getenv("NETQUASAR_DB_HOST", "127.0.0.1"),
		DBPort:           mustAtoi(getenv("NETQUASAR_DB_PORT", "5432"), 5432),
		DBUser:           getenv("NETQUASAR_DB_USER", "postgres"),
		DBPassword:       os.Getenv("NETQUASAR_DB_PASSWORD"),
		DBName:           getenv("NETQUASAR_DB_NAME", "netquasar"),
		DBSSLMode:        getenv("NETQUASAR_DB_SSLMODE", "disable"),
		DBSSLRootCert:    strings.TrimSpace(os.Getenv("NETQUASAR_DB_SSLROOTCERT")),
		DatabaseURL:      os.Getenv("NETQUASAR_DATABASE_URL"),
	}

	if cors := os.Getenv("NETQUASAR_CORS_ORIGINS"); cors != "" {
		// vírgula-separado
		for _, o := range splitComma(cors) {
			if o != "" {
				c.CORSOrigins = append(c.CORSOrigins, o)
			}
		}
	}
	if keys := os.Getenv("NETQUASAR_API_KEYS"); keys != "" {
		c.APIKeys = splitComma(keys)
	}
	c.SessionSecret = strings.TrimSpace(os.Getenv("NETQUASAR_SESSION_SECRET"))
	c.EmbeddedUI = parseBoolEnv("NETQUASAR_EMBEDDED_UI", false)

	if c.DBSSLRootCert != "" {
		if _, err := os.Stat(c.DBSSLRootCert); err != nil {
			return nil, fmt.Errorf("NETQUASAR_DB_SSLROOTCERT (%s): %w", c.DBSSLRootCert, err)
		}
	}
	return c, nil
}

// PostgresURLFromParts monta DSN postgres:// com sslmode (uso em settings/database e testes).
func PostgresURLFromParts(host string, port int, user, password, database, sslMode string) string {
	if sslMode == "" {
		sslMode = "disable"
	}
	if port <= 0 {
		port = 5432
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/" + database,
	}
	q := u.Query()
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()
	return u.String()
}

// AppendSSLRootCertToDSN acrescenta o parâmetro sslrootcert à connection string (libpq / pgx).
// certPath deve ser um caminho existente para um ficheiro PEM (ex.: CA raiz da Supabase).
func AppendSSLRootCertToDSN(dsn, certPath string) (string, error) {
	certPath = strings.TrimSpace(certPath)
	if certPath == "" {
		return dsn, nil
	}
	abs, err := filepath.Abs(certPath)
	if err != nil {
		return "", fmt.Errorf("sslrootcert path: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("sslrootcert: %w", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse dsn: %w", err)
	}
	q := u.Query()
	if q.Get("sslrootcert") != "" {
		return dsn, nil
	}
	// Caminho absoluto normalizado; em Windows pgx/libpq aceita barras na query após codificação.
	q.Set("sslrootcert", filepath.ToSlash(abs))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// supabaseCAFileCandidates devolve caminhos possíveis para o PEM da raiz Supabase (empacotado no repo).
func supabaseCAFileCandidates() []string {
	var out []string
	if e := strings.TrimSpace(os.Getenv("NETQUASAR_DB_SSLROOTCERT")); e != "" {
		out = append(out, e)
	}
	out = append(out, "data/certs/supabase-root-ca-2021.pem")
	// Imagem Docker (WORKDIR /app); em Windows local este caminho não existe e é ignorado.
	out = append(out, "/app/data/certs/supabase-root-ca-2021.pem")
	return out
}

func tryStatCertPath(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", false
	}
	if filepath.IsAbs(p) {
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		return "", false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	if _, err := os.Stat(abs); err == nil {
		return abs, true
	}
	return "", false
}

// EnsureSupabaseSSLRootCertIfNeeded acrescenta sslrootcert quando o host é db.*.supabase.co (ligação direta) e o PEM existe.
// Resolve testes de ligação a partir do Docker (sem NETQUASAR_DB_SSLROOTCERT no ambiente) e a partir do Windows.
func EnsureSupabaseSSLRootCertIfNeeded(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	host := strings.ToLower(u.Hostname())
	// Só ligação direta db.*.supabase.co — não confundir com *.pooler.supabase.com
	// (strings.Contains(..., "supabase.co") acertava em "pooler.supabase.com" por engano).
	if !IsSupabaseDirectDBHost(host) {
		return dsn
	}
	q := u.Query()
	if q.Get("sslrootcert") != "" {
		return dsn
	}
	sm := strings.ToLower(q.Get("sslmode"))
	if sm == "" {
		q.Set("sslmode", "require")
		u.RawQuery = q.Encode()
		dsn = u.String()
	}
	for _, cand := range supabaseCAFileCandidates() {
		abs, ok := tryStatCertPath(cand)
		if !ok {
			continue
		}
		out, err := AppendSSLRootCertToDSN(dsn, abs)
		if err == nil {
			return out
		}
	}
	return dsn
}

// ConfigFromPostgresDSN retorna Config mínimo cuja PostgresDSN() é apenas a URL informada.
func ConfigFromPostgresDSN(dsn string) *Config {
	return &Config{DatabaseURL: dsn}
}

func (c *Config) sslRootCertPath() string {
	if c.DBSSLRootCert != "" {
		return c.DBSSLRootCert
	}
	return strings.TrimSpace(os.Getenv("NETQUASAR_DB_SSLROOTCERT"))
}

// explicitSSLRootCertForBase aplica NETQUASAR_DB_SSLROOTCERT / DBSSLRootCert excepto no host do pooler partilhado,
// onde o PEM da instância Postgres da Supabase não corresponde ao certificado TLS do endpoint AWS.
func (c *Config) explicitSSLRootCertForBase(base string) string {
	path := c.sslRootCertPath()
	if path == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil {
		return path
	}
	h := strings.ToLower(u.Hostname())
	if strings.HasSuffix(h, ".pooler.supabase.com") {
		return ""
	}
	return path
}

func (c *Config) PostgresDSN() string {
	var base string
	if c.DatabaseURL != "" {
		base = c.DatabaseURL
	} else {
		base = PostgresURLFromParts(c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode)
	}
	out, err := AppendSSLRootCertToDSN(base, c.explicitSSLRootCertForBase(base))
	if err != nil {
		out = base
	}
	out = EnsureSupabaseSSLRootCertIfNeeded(out)
	return out
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			seg := trimSpace(s[start:i])
			if seg != "" {
				out = append(out, seg)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
