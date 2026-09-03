-- +goose Up
-- Perfil OLT VSOL V1600G0B (Definições → Perfis OLT, olt_vendor_models.onu_metrics) tinha o OID
-- do campo "serial" configurado por engano igual ao do campo "status" + ".2" no fim
-- (1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.2), em vez do OID real de serial que todos os outros
-- perfis VSOL usam (1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5) — a tabela de status devolve um código
-- de fase pequeno (3=working), gravado como se fosse o serial da ONU ("serial vira '3'" nas
-- ONUs dessa OLT, e por tabela errada o cliente vinculado por serial nunca batia).
--
-- Corrige só se a linha existir E ainda tiver exactamente o valor errado conhecido — não mexe em
-- nada que o utilizador possa ter configurado/corrigido manualmente por conta própria depois.
-- Complementa (não substitui) o guard de código em internal/oltcollect/onu_serial_guard.go, que
-- passa a impedir esse tipo de valor implausível de ser gravado independentemente da causa.
UPDATE olt_vendor_models
SET onu_metrics = jsonb_set(onu_metrics, '{serial,oid}', '"1.3.6.1.4.1.37950.1.1.6.1.1.2.1.5"'::jsonb),
    updated_at = now()
WHERE brand = 'VSOL' AND model = 'V1600G0B'
  AND onu_metrics -> 'serial' ->> 'oid' = '1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.2';

-- Limpeza complementar: remove seriais já gravados por essa configuração errada (menos de 8
-- caracteres — todo serial de ONU real tem 12) de qualquer OLT, não só V1600G0B, para o caso do
-- mesmo tipo de engano existir noutro perfil que ainda não identificámos. Não apaga a linha da
-- ONU, só o campo "serial" implausível — a próxima coleta preenche de novo, agora com o OID
-- certo (ou fica em branco em vez de errado, se a causa for outra).
WITH cleaned AS (
    SELECT
        os.device_id,
        jsonb_set(
            os.summary,
            '{vsol_onu_rows}',
            (
                SELECT jsonb_agg(
                    CASE WHEN length(elem ->> 'serial') < 8 THEN elem - 'serial' ELSE elem END
                    ORDER BY ord
                )
                FROM jsonb_array_elements(os.summary -> 'vsol_onu_rows') WITH ORDINALITY AS t(elem, ord)
            )
        ) AS new_summary
    FROM olt_snapshots os
    WHERE os.summary ? 'vsol_onu_rows'
      AND EXISTS (
          SELECT 1 FROM jsonb_array_elements(os.summary -> 'vsol_onu_rows') e2
          WHERE length(e2 ->> 'serial') < 8
      )
)
UPDATE olt_snapshots os
SET summary = cleaned.new_summary
FROM cleaned
WHERE os.device_id = cleaned.device_id;

-- +goose Down
-- Não há como reverter com segurança (não sabíamos o serial correcto de cada ONU limpa) —
-- migração só de correcção de dados, sem down funcional.
