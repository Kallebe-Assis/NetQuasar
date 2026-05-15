import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Moon, Sun } from "lucide-react";
import { useEffect, useState } from "react";
import { useUiTheme } from "../../app/ThemeProvider";
import { InfoHint } from "../../components/InfoHint";
import { useThemePreview } from "../../hooks/useThemePreview";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";
import { applyUiTheme, uiThemeLabel, type UiTheme } from "../../lib/theme";
import { fetchUiAppearance, normalizeUiAppearanceCacheValue, themeFromAppearancePayload } from "../../lib/uiAppearance";

export function AppearancePanel() {
  const qc = useQueryClient();
  const { theme: activeTheme } = useUiTheme();
  const q = useQuery({
    queryKey: queryKeys.uiAppearance,
    queryFn: fetchUiAppearance,
  });
  const [draft, setDraft] = useState<UiTheme>(activeTheme);
  const [saveToast, setSaveToast] = useState<{ ok: boolean; text: string } | null>(null);

  useThemePreview(draft, activeTheme);

  const appearance = normalizeUiAppearanceCacheValue(q.data);

  useEffect(() => {
    setDraft(themeFromAppearancePayload(appearance?.theme, activeTheme));
  }, [appearance?.theme, activeTheme]);

  const save = useMutation({
    mutationFn: (theme: UiTheme) =>
      apiFetch<{ ok?: boolean; theme?: string }>("/api/v1/settings/ui-appearance", {
        method: "PATCH",
        json: { theme },
      }),
    onSuccess: (_data, theme) => {
      applyUiTheme(theme);
      qc.setQueryData(queryKeys.uiAppearance, { theme, updated_at: new Date().toISOString() });
      void qc.invalidateQueries({ queryKey: queryKeys.uiAppearance });
      setSaveToast({ ok: true, text: `Tema «${uiThemeLabel(theme)}» guardado para todos os utilizadores.` });
    },
    onError: (e: Error) => setSaveToast({ ok: false, text: e.message }),
  });

  const options: { id: UiTheme; title: string; hint: string; icon: typeof Sun }[] = [
    { id: "dark", title: "Escuro", hint: "Fundo escuro, texto claro — predefinido operacional.", icon: Moon },
    { id: "light", title: "Claro", hint: "Fundo claro, texto escuro — melhor em ambientes iluminados.", icon: Sun },
  ];

  return (
    <div className="panel" style={{ maxWidth: 560 }}>
      <h2 style={{ marginTop: 0, display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6 }}>
        Tema da interface
        <InfoHint label="Tema claro e escuro">
          <p>
            Define o aspecto visual de todo o NetQuasar (menu, tabelas, alertas, login). A preferência fica guardada na base de dados e aplica-se a todos os
            utilizadores.
          </p>
        </InfoHint>
      </h2>
      {q.isLoading ? <p style={{ color: "var(--muted)" }}>A carregar…</p> : null}
      <div className="theme-picker" role="radiogroup" aria-label="Tema da interface">
        {options.map((opt) => {
          const Icon = opt.icon;
          const active = draft === opt.id;
          return (
            <button
              key={opt.id}
              type="button"
              role="radio"
              aria-checked={active}
              className={`theme-picker__option${active ? " theme-picker__option--active" : ""}`}
              onClick={() => setDraft(opt.id)}
            >
              <span className="theme-picker__icon" aria-hidden>
                <Icon size={22} strokeWidth={2} />
              </span>
              <span className="theme-picker__title">{opt.title}</span>
              <span className="theme-picker__hint">{opt.hint}</span>
            </button>
          );
        })}
      </div>
      <div className="row" style={{ marginTop: 16, gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <button type="button" className="btn btn--primary" disabled={save.isPending || q.isLoading} onClick={() => save.mutate(draft)}>
          Guardar tema
        </button>
        <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>
          Pré-visualização: <strong style={{ color: "var(--text)" }}>{uiThemeLabel(draft)}</strong> (restaura ao sair sem guardar)
        </p>
      </div>
      {saveToast ? (
        <div className={`page-toast ${saveToast.ok ? "page-toast--ok" : "page-toast--err"}`} role="status" style={{ marginTop: 10 }}>
          <button type="button" className="page-toast__close" aria-label="Fechar" onClick={() => setSaveToast(null)}>
            ×
          </button>
          {saveToast.text}
        </div>
      ) : null}
    </div>
  );
}