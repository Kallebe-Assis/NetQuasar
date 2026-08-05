-- Auditoria de execuções: utilizador que disparou (manual) + índice por origem.
ALTER TABLE automation_execution_log
  ADD COLUMN IF NOT EXISTS triggered_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS automation_execution_log_trigger_started_idx
  ON automation_execution_log (trigger_type, started_at DESC);

CREATE INDEX IF NOT EXISTS automation_execution_log_user_started_idx
  ON automation_execution_log (triggered_by_user_id, started_at DESC)
  WHERE triggered_by_user_id IS NOT NULL;
