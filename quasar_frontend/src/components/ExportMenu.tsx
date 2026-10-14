import { Download } from "lucide-react";
import { useState } from "react";
import { DropdownMenu } from "./DropdownMenu";
import { useAppToast } from "../lib/appToast";
import { EXPORT_FORMAT_LABEL, exportTable, type ExportColumn, type ExportFormat } from "../lib/tableExport";

type Props<T> = {
  /** Nome do arquivo (sem extensão); a data/hora é acrescentada. */
  filename: string;
  /** Título (planilha e cabeçalho do PDF). */
  title: string;
  columns: ExportColumn<T>[];
  /** Linhas a exportar — já filtradas como na tela. Pode ser uma função (calculada só ao exportar). */
  rows: T[] | (() => T[] | Promise<T[]>);
  disabled?: boolean;
  className?: string;
};

const FORMATS: ExportFormat[] = ["csv", "xlsx", "pdf"];

/** Botão «Exportar» (CSV, Excel e PDF) para qualquer lista da tela. */
export function ExportMenu<T>({ filename, title, columns, rows, disabled, className = "btn btn--icon btn--icon-menu" }: Props<T>) {
  const { push } = useAppToast();
  const [busy, setBusy] = useState(false);

  async function run(format: ExportFormat) {
    setBusy(true);
    try {
      const list = typeof rows === "function" ? await rows() : rows;
      if (list.length === 0) {
        push({ tone: "warn", text: "Não há linhas para exportar com o filtro atual." });
        return;
      }
      // deixa o botão atualizar antes do trabalho pesado (PDF/XLSX grandes)
      await new Promise((r) => setTimeout(r, 0));
      exportTable(format, { filename, title, columns, rows: list });
      push({ tone: "ok", text: `${list.length.toLocaleString("pt-BR")} linha(s) exportada(s) em ${EXPORT_FORMAT_LABEL[format]}.` });
    } catch (e) {
      push({ tone: "err", text: `Falha ao exportar: ${e instanceof Error ? e.message : String(e)}` });
    } finally {
      setBusy(false);
    }
  }

  return (
    <DropdownMenu
      align="end"
      className="action-menu"
      trigger={({ toggle, open }) => (
        <button
          type="button"
          className={className}
          disabled={disabled || busy}
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={toggle}
          title="Exportar (CSV, Excel ou PDF)"
          aria-label="Exportar (CSV, Excel ou PDF)"
        >
          <Download size={18} aria-hidden />
        </button>
      )}
    >
      {({ close }) =>
        FORMATS.map((f) => (
          <button
            key={f}
            type="button"
            className="action-menu__item"
            onClick={() => {
              close();
              void run(f);
            }}
          >
            {EXPORT_FORMAT_LABEL[f]}
          </button>
        ))
      }
    </DropdownMenu>
  );
}
