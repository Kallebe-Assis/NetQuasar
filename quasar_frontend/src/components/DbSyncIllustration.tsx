import { useId, useState, useEffect } from "react";

function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReduced(mq.matches);
    const onChange = () => setReduced(mq.matches);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);
  return reduced;
}

/**
 * Pequena ilustração decorativa "Backend ⇄ Postgres" para a aba Base de dados.
 * Usa só tokens de tema (var(--accent)/--ok/--border/--muted/--panel2) para se
 * adaptar automaticamente ao tema claro/escuro — sem cores fixas.
 */
export function DbSyncIllustration() {
  const gradId = useId();
  const reducedMotion = usePrefersReducedMotion();

  return (
    <svg
      viewBox="0 0 220 96"
      width="200"
      height="88"
      role="img"
      aria-label="Sincronização entre o backend NetQuasar e a base de dados Postgres"
    >
      <defs>
        <linearGradient id={`${gradId}-line`} x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.15" />
          <stop offset="50%" stopColor="var(--accent)" stopOpacity="0.9" />
          <stop offset="100%" stopColor="var(--accent)" stopOpacity="0.15" />
        </linearGradient>
      </defs>

      {/* Linha de ligação */}
      <path
        id={`${gradId}-path`}
        d="M 60 48 H 160"
        fill="none"
        stroke={`url(#${gradId}-line)`}
        strokeWidth="2"
        strokeDasharray="4 5"
      />

      {/* Pulso animado a percorrer a ligação (desligado se prefers-reduced-motion) */}
      {!reducedMotion && (
        <circle r="3.5" fill="var(--ok)">
          <animateMotion dur="2.4s" repeatCount="indefinite" path="M 60 48 H 160" />
        </circle>
      )}

      {/* Nó "Backend" */}
      <g>
        <rect x="8" y="26" width="52" height="44" rx="8" fill="var(--panel2)" stroke="var(--border)" />
        <rect x="18" y="36" width="32" height="6" rx="2" fill="var(--muted)" />
        <rect x="18" y="46" width="32" height="6" rx="2" fill="var(--muted)" opacity="0.7" />
        <rect x="18" y="56" width="20" height="6" rx="2" fill="var(--muted)" opacity="0.5" />
      </g>
      <text x="34" y="86" textAnchor="middle" fontSize="9" fill="var(--muted)">
        Backend
      </text>

      {/* Nó "Postgres" (cilindro) */}
      <g>
        <ellipse cx="186" cy="34" rx="26" ry="8" fill="var(--panel2)" stroke="var(--border)" />
        <path d="M 160 34 V 62 A 26 8 0 0 0 212 62 V 34" fill="var(--panel2)" stroke="var(--border)" />
        <ellipse cx="186" cy="62" rx="26" ry="8" fill="none" stroke="var(--border)" />
      </g>
      <text x="186" y="86" textAnchor="middle" fontSize="9" fill="var(--muted)">
        Postgres
      </text>
    </svg>
  );
}
