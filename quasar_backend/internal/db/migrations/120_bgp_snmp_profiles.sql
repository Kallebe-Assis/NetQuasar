-- +goose Up
-- Perfis SNMP de BGP (Configurações → BGP) — mesmo padrão de mikrotik_telnet_profiles (071):
-- vários perfis nomeados, um marcado como padrão. O conteúdo de OIDs de cada perfil vem do
-- catálogo em internal/bgpcollect (saúde/interfaces/peers/tráfego) — o perfil "Padrão" nasce
-- vazio aqui (como o dos outros perfis) e é pré-populado no primeiro carregamento pela própria
-- UI/API a partir do catálogo, igual ao que já acontece com BNG/Mikrotik/Switch.
CREATE TABLE IF NOT EXISTS bgp_snmp_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    metrics JSONB NOT NULL DEFAULT '{}',
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bgp_snmp_profiles_name
    ON bgp_snmp_profiles (lower(trim(name)));

INSERT INTO bgp_snmp_profiles (name, metrics, is_default)
SELECT 'Padrão', '{}'::jsonb, true
WHERE NOT EXISTS (SELECT 1 FROM bgp_snmp_profiles WHERE is_default = true);

-- +goose Down
DROP TABLE IF EXISTS bgp_snmp_profiles;
