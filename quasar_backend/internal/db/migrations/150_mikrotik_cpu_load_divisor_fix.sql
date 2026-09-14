-- +goose Up
-- cpu_load (mtxrHlCpuLoad) tinha um divisor 10 por defeito, copiado por engano do padrão dos
-- sensores de temperatura/voltagem — mas essa OID já vem em percentagem directa (0-100), sem
-- factor de escala. Isso fazia o CPU aparecer a 1/10 do valor real (ex.: 45% real → "4.5%" na
-- tela). Corrige só quem ainda tem exactamente o divisor antigo (nunca editado à mão desde
-- então) — não mexe em quem já personalizou o divisor deliberadamente.
UPDATE settings_mikrotik_collection
SET metrics = jsonb_set(
    metrics,
    '{cpu_load,value_divisor}',
    '0'::jsonb
)
WHERE id = 1
  AND metrics -> 'cpu_load' ->> 'value_divisor' = '10';

-- +goose Down
-- Não reversível de forma segura (não dá para distinguir "estava no divisor antigo por engano"
-- de "o utilizador voltou a pôr 10 à mão" depois do Up) — down é no-op.
