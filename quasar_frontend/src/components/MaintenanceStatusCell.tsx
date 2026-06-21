/** Sem manutenção pendente: ✕ verde (positivo). Com pendência: ✓ vermelho. */
export function MaintenanceStatusCell({ needsMaintenance }: { needsMaintenance: boolean }) {
  if (needsMaintenance) {
    return (
      <span title="Manutenção pendente" style={{ color: "#dc2626", fontWeight: 700, fontSize: 16, lineHeight: 1 }}>
        ✓
      </span>
    );
  }
  return (
    <span title="Sem manutenção pendente" style={{ color: "#16a34a", fontWeight: 700, fontSize: 16, lineHeight: 1 }}>
      ✕
    </span>
  );
}
