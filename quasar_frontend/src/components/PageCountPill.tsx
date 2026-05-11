export function PageCountPill({ label, count }: { label: string; count: number }) {
  return (
    <span className="page-count-pill" title={`Total de ${label}`}>
      {label}: <span className="mono">{count}</span>
    </span>
  );
}

