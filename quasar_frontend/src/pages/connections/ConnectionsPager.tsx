type Props = {
  safePage: number;
  totalPages: number;
  onPrev: () => void;
  onNext: () => void;
  rangeFrom?: number;
  rangeTo?: number;
  total?: number;
};

export function ConnectionsPager({ safePage, totalPages, onPrev, onNext, rangeFrom, rangeTo, total }: Props) {
  if (totalPages <= 1 && (total ?? 0) <= 0) return null;
  return (
    <div className="conn-table-pager row" style={{ marginTop: 10, gap: 8, alignItems: "center", flexWrap: "wrap" }}>
      <button type="button" className="btn" disabled={safePage <= 0} onClick={onPrev}>
        Anterior
      </button>
      <span style={{ fontSize: 12, color: "var(--muted)" }}>
        Página {safePage + 1} / {totalPages}
        {rangeFrom != null && rangeTo != null && total != null && total > 0
          ? ` · ${rangeFrom}–${rangeTo} de ${total}`
          : null}
      </span>
      <button type="button" className="btn" disabled={safePage >= totalPages - 1} onClick={onNext}>
        Seguinte
      </button>
    </div>
  );
}
