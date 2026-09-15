-- +goose Up
-- Estado singleton da atualização do sistema pelo GitHub (aba "Sobre" → "Versão do sistema").
-- O container netquasar não tem acesso a Docker/git do host (de propósito — ver serviço
-- `updater` dedicado em docker-compose.yml); esta tabela é o canal de comunicação entre ele
-- (que só grava pedidos) e o `updater` (que faz polling e executa git pull + rebuild + restart).
CREATE TABLE IF NOT EXISTS system_update_state (
    id                        INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    status                    TEXT NOT NULL DEFAULT 'idle',
    -- idle | check_requested | checking | check_done | update_requested | updating | completed | failed
    base_commit               TEXT,
    branch                    TEXT NOT NULL DEFAULT 'main',
    requested_at              TIMESTAMPTZ,
    requested_by              TEXT,
    previous_is_running       BOOLEAN,
    previous_monitoring_mode  TEXT,
    check_result              JSONB,
    started_at                TIMESTAMPTZ,
    completed_at              TIMESTAMPTZ,
    error_message             TEXT,
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO system_update_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS system_update_state;
