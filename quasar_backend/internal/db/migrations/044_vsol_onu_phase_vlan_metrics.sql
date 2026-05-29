-- +goose Up
-- VSOL: fase gOnuStaInfoPhaseSta (…1.1.1.1.5.1.{onu}); valores working=3, offline los/dyingGasp/etc.
-- Campo VLAN (gOnuCfgPortVlanDefVlan) desactivado por defeito — activar no perfil do modelo.
UPDATE olt_vendor_models SET
  onu_metrics = onu_metrics
    || '{"vlan":{"enabled":false,"oid":"1.3.6.1.4.1.37950.1.1.6.1.1.7.5.8"}}'::jsonb
    || '{"status":{"enabled":true,"oid":"1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5","online_values":[3],"offline_values":[0,1,2,4,5,6]}}'::jsonb
WHERE brand = 'VSOL';

-- +goose Down
UPDATE olt_vendor_models SET
  onu_metrics = onu_metrics - 'vlan'
WHERE brand = 'VSOL';

UPDATE olt_vendor_models SET
  onu_metrics = jsonb_set(
    onu_metrics,
    '{status}',
    '{"enabled":true,"oid":"1.3.6.1.4.1.37950.1.1.6.1.1.1.1.5.2","online_values":[3],"offline_values":[4]}'::jsonb
  )
WHERE brand = 'VSOL';
