-- +goose Up
-- Permite a um admin forçar a desconexão de outro usuário. A autenticação é por JWT sem estado
-- (sem tabela de sessões) — em vez disso, guardamos aqui o instante a partir do qual qualquer
-- token emitido ANTES desse instante deixa de ser aceite (comparado com o "issued at" do JWT em
-- requestAuthContext, internal/api/authz.go). O próximo pedido autenticado do usuário-alvo
-- recebe 401 e o frontend redireciona para o login (ver apiFetch em quasar_frontend/src/lib/api.ts).
ALTER TABLE users ADD COLUMN sessions_invalidated_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS sessions_invalidated_at;
