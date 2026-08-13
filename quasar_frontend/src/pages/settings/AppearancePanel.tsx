import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Moon, Sun } from "lucide-react";
import { useEffect, useState } from "react";
import { useUiTheme } from "../../app/ThemeProvider";
import { InfoHint } from "../../components/InfoHint";
import { useThemePreview } from "../../hooks/useThemePreview";
import { queryKeys } from "../../lib/queryKeys";
import { applyUiTheme, uiThemeLabel, type UiTheme } from "../../lib/theme";
import { fetchMyPreferences, patchMyPreferences } from "../../lib/userPreferences";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";

export function AppearancePanel() {
  const qc = useQueryClient();
  const { theme: activeTheme } = useUiTheme();
  const q = useQuery({
    queryKey: queryKeys.mePreferences,
    queryFn: fetchMyPreferences,
  });
  const [draft, setDraft] = useState<UiTheme>(activeTheme);
  const { push: pushToast } = useAppToast();

  useThemePreview(draft, activeTheme);

  useEffect(() => {
    if (q.data?.theme) setDraft(q.data.theme);
  }, [q.data?.theme]);

  const save = useMutation({
    mutationFn: (theme: UiTheme) => patchMyPreferences({ theme }),
    onSuccess: (next) => {
      applyUiTheme(next.theme);
      qc.setQueryData(queryKeys.mePreferences, next);
      toastOk(pushToast, `Tema «${uiThemeLabel(next.theme)}» guardado só para a sua conta.`);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao salvar tema."),
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
            Define o aspecto visual do NetQuasar neste utilizador (menu, tabelas, alertas). Cada conta guarda o seu
            próprio tema — não altera o dos colegas.
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
          Salvar tema
        </button>
        <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>
          Pré-visualização: <strong style={{ color: "var(--text)" }}>{uiThemeLabel(draft)}</strong> (restaura ao sair sem salvar)
        </p>
      </div>
    </div>
  );
}
