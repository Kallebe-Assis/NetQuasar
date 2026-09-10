import type { ReactNode } from "react";

type Props = {
  /** Frase curta — o que falta. Ex.: "Sem histórico ainda para esta ONU." */
  title: string;
  /** Uma linha a explicar o porquê / o que fazer. Opcional. */
  hint?: ReactNode;
  /** Ação opcional (botão "Atualizar", link para configuração, etc.). */
  action?: ReactNode;
  /** Ícone opcional à esquerda do título (ex.: um lucide icon já dimensionado). */
  icon?: ReactNode;
  /** "inline" = parágrafo simples (dentro de um card já existente); "block" = caixa tracejada
   * centrada (quando é o único conteúdo da secção). Default "block". */
  variant?: "inline" | "block";
};

/**
 * Estado vazio padrão do NetQuasar — um só componente para "ainda não há dados aqui", em vez de
 * cada tela inventar o seu (umas explicavam bem, outras mostravam só um traço ou um gráfico em
 * branco). Sempre diz O QUE falta e, quando dá, PORQUÊ / o que fazer a seguir.
 */
export function EmptyState({ title, hint, action, icon, variant = "block" }: Props) {
  if (variant === "inline") {
    return (
      <p style={{ fontSize: 12, color: "var(--muted)", margin: 0, display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
        {icon}
        <span>{title}</span>
        {hint ? <span style={{ opacity: 0.85 }}>— {hint}</span> : null}
        {action}
      </p>
    );
  }
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 8,
        textAlign: "center",
        padding: "26px 18px",
        border: "1px dashed var(--border)",
        borderRadius: 10,
        color: "var(--muted)",
        background: "color-mix(in srgb, var(--panel2) 45%, transparent)",
      }}
    >
      {icon ? <div style={{ opacity: 0.7 }}>{icon}</div> : null}
      <div style={{ fontSize: 13.5, fontWeight: 500, color: "var(--text)" }}>{title}</div>
      {hint ? <div style={{ fontSize: 12, maxWidth: 460, lineHeight: 1.5 }}>{hint}</div> : null}
      {action ? <div style={{ marginTop: 4 }}>{action}</div> : null}
    </div>
  );
}
