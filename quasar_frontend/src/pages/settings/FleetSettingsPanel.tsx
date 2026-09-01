import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { apiFetch, downloadBlob } from "../../lib/api";
import { apiUrl, can, getAuthToken, getStoredApiKey, isAdminUser } from "../../lib/auth";
import { InfoHint } from "../../components/InfoHint";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastInfo, toastOk } from "../../lib/operationToast";
import { buildExcelCsvBlob } from "../../lib/excelCsv";
import { invalidateFleetOperationalQueries, queryKeys } from "../../lib/queryKeys";

type FleetSettings = {
  consumption_tolerance_pct: number;
  price_tolerance_pct: number;
  min_minutes_between_fuelings: number;
};

type CsvImportResult = {
  imported: number;
  failed?: { line: number; error: string }[];
};

const VEHICLE_CSV_HEADER = [
  "descricao",
  "placa",
  "ano",
  "modelo",
  "cor",
  "cidade",
  "uf",
  "combustivel",
  "capacidade_tanque_l",
  "consumo_esperado_km_l",
  "consumo_min_km_l",
  "consumo_max_km_l",
  "hodometro",
  "centro_custo",
  "status",
  "observacao",
];

const VEHICLE_CSV_SAMPLE = [
  "Hilux Manutenção",
  "ABC-1D23",
  "2024",
  "Hilux",
  "Branca",
  "Miracema",
  "RJ",
  "Diesel S10",
  "80",
  "10",
  "8",
  "12",
  "142520",
  "003",
  "ativo",
  "",
];

const DRIVER_CSV_HEADER = [
  "nome",
  "cpf",
  "rg",
  "telefone",
  "email",
  "cnh",
  "categoria_cnh",
  "validade_cnh",
  "cidade",
  "uf",
  "usuario",
  "status",
  "observacao",
];

const DRIVER_CSV_SAMPLE = [
  "João da Silva",
  "12345678901",
  "",
  "21999999999",
  "joao@empresa.com",
  "01234567890",
  "AB",
  "2028-12-31",
  "Miracema",
  "RJ",
  "",
  "ativo",
  "",
];

const EXPENSE_CSV_HEADER = [
  "lancamento",
  "descricao",
  "placa",
  "data",
  "tipo_despesa",
  "valor_unitario",
  "quantidade",
  "valor",
  "km",
  "observacao",
];

const EXPENSE_CSV_SAMPLE = [
  "despesa",
  "Troca de óleo",
  "ABC-1D23",
  "2024-03-15",
  "manutenção preventiva",
  "85,90",
  "1",
  "85,90",
  "142520",
  "Revisão 10 mil",
];

const EXPENSE_CSV_SAMPLE_FUEL = [
  "abastecimento",
  "Diesel S10",
  "ABC-1D23",
  "2024-03-16",
  "Diesel S10",
  "6,19",
  "40",
  "247,60",
  "142800",
  "Tanque cheio",
];

async function importFleetCsv(path: string, file: File): Promise<CsvImportResult> {
  const form = new FormData();
  form.append("file", file);
  const headers = new Headers();
  const token = getAuthToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const key = getStoredApiKey();
  if (key) headers.set("X-API-Key", key);
  const res = await fetch(apiUrl(path), { method: "POST", headers, body: form });
  const data = (await res.json().catch(() => ({}))) as {
    imported?: number;
    failed?: { line: number; error: string }[];
    error?: string | { message?: string };
  };
  if (!res.ok) {
    const msg =
      typeof data.error === "string"
        ? data.error
        : data.error?.message || `Falha ao importar CSV (${res.status})`;
    throw new Error(msg);
  }
  return { imported: data.imported ?? 0, failed: data.failed ?? [] };
}

export function FleetSettingsPanel() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const vehicleFileRef = useRef<HTMLInputElement>(null);
  const driverFileRef = useRef<HTMLInputElement>(null);
  const expenseFileRef = useRef<HTMLInputElement>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [vehicleFailed, setVehicleFailed] = useState<{ line: number; error: string }[]>([]);
  const [driverFailed, setDriverFailed] = useState<{ line: number; error: string }[]>([]);
  const [expenseFailed, setExpenseFailed] = useState<{ line: number; error: string }[]>([]);
  const [wipeBackup, setWipeBackup] = useState(true);
  const [wipeStep, setWipeStep] = useState<"idle" | "confirm">("idle");
  const [wipeConfirm, setWipeConfirm] = useState("");

  const q = useQuery({
    queryKey: queryKeys.fleetSettings,
    queryFn: () => apiFetch<FleetSettings>("/api/v1/fleet/settings"),
  });
  const [form, setForm] = useState<FleetSettings>({
    consumption_tolerance_pct: 20,
    price_tolerance_pct: 15,
    min_minutes_between_fuelings: 60,
  });

  useEffect(() => {
    if (q.data) setForm(q.data);
  }, [q.data]);

  const save = useMutation({
    mutationFn: () =>
      apiFetch("/api/v1/fleet/settings", {
        method: "PATCH",
        body: JSON.stringify({
          consumption_tolerance_pct: Number(form.consumption_tolerance_pct),
          price_tolerance_pct: Number(form.price_tolerance_pct),
          min_minutes_between_fuelings: Number(form.min_minutes_between_fuelings),
        }),
      }),
    onSuccess: async () => {
      toastOk(push, "Configurações de frota guardadas.");
      await qc.invalidateQueries({ queryKey: queryKeys.fleetSettings });
    },
    onError: (e) => toastErr(push, e, "Falha ao guardar configurações de frota."),
  });

  const importVehicles = useMutation({
    mutationFn: (file: File) => importFleetCsv("/api/v1/fleet/vehicles/import/csv", file),
    onSuccess: async (data) => {
      await qc.invalidateQueries({ queryKey: queryKeys.fleetVehicles });
      const failed = data.failed ?? [];
      setVehicleFailed(failed);
      if (failed.length > 0) {
        toastInfo(
          push,
          `Importação parcial de veículos: ${data.imported} importado(s), ${failed.length} falha(s).`,
        );
      } else {
        toastOk(push, `Importação de veículos concluída: ${data.imported} item(ns).`);
      }
    },
    onError: (e) => toastErr(push, e, "Falha ao importar veículos."),
  });

  const importDrivers = useMutation({
    mutationFn: (file: File) => importFleetCsv("/api/v1/fleet/drivers/import/csv", file),
    onSuccess: async (data) => {
      await qc.invalidateQueries({ queryKey: queryKeys.fleetDrivers });
      const failed = data.failed ?? [];
      setDriverFailed(failed);
      if (failed.length > 0) {
        toastInfo(
          push,
          `Importação parcial de motoristas: ${data.imported} importado(s), ${failed.length} falha(s).`,
        );
      } else {
        toastOk(push, `Importação de motoristas concluída: ${data.imported} item(ns).`);
      }
    },
    onError: (e) => toastErr(push, e, "Falha ao importar motoristas."),
  });

  const importExpenses = useMutation({
    mutationFn: (file: File) => importFleetCsv("/api/v1/fleet/expenses/import/csv", file),
    onSuccess: async (data) => {
      await invalidateFleetOperationalQueries(qc);
      const failed = data.failed ?? [];
      setExpenseFailed(failed);
      if (failed.length > 0) {
        toastInfo(
          push,
          `Importação parcial de despesas: ${data.imported} importado(s), ${failed.length} falha(s).`,
        );
      } else {
        toastOk(push, `Importação de despesas concluída: ${data.imported} item(ns).`);
      }
    },
    onError: (e) => toastErr(push, e, "Falha ao importar despesas."),
  });

  async function downloadExpensesBackup() {
    const headers = new Headers();
    const token = getAuthToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
    const key = getStoredApiKey();
    if (key) headers.set("X-API-Key", key);
    const res = await fetch(apiUrl("/api/v1/fleet/expenses/export/csv"), { headers });
    if (!res.ok) throw new Error(`Falha no backup (${res.status})`);
    const blob = await res.blob();
    downloadBlob(`backup_despesas_${new Date().toISOString().slice(0, 10)}.csv`, blob);
  }

  const purgeExpenses = useMutation({
    mutationFn: async () => {
      if (wipeBackup) await downloadExpensesBackup();
      return apiFetch<{ deleted_expenses: number; deleted_fuelings: number }>("/api/v1/fleet/expenses/purge", {
        method: "POST",
        json: { confirm: "ZERAR" },
      });
    },
    onSuccess: async (data) => {
      toastOk(push, `Base zerada: ${data.deleted_expenses} despesa(s) e ${data.deleted_fuelings} abastecimento(s) apagados.`);
      setWipeStep("idle");
      setWipeConfirm("");
      await invalidateFleetOperationalQueries(qc);
    },
    onError: (e) => toastErr(push, e, "Falha ao apagar despesas."),
  });

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div className="card">
        <h2>Frota</h2>
        <p style={{ color: "var(--muted)", marginTop: 0 }}>
          Regras usadas nos abastecimentos para detectar consumo anormal, preço fora do padrão e lançamentos muito próximos.
        </p>
        {q.isLoading ? <p className="muted">A carregar…</p> : null}
        <div className="fleet-form-grid">
          <label>
            Tolerância de consumo (%)
            <input
              className="input"
              type="number"
              min={0}
              step={1}
              value={form.consumption_tolerance_pct}
              onChange={(e) => setForm({ ...form, consumption_tolerance_pct: Number(e.target.value) })}
            />
          </label>
          <label>
            Tolerância de preço (%)
            <input
              className="input"
              type="number"
              min={0}
              step={1}
              value={form.price_tolerance_pct}
              onChange={(e) => setForm({ ...form, price_tolerance_pct: Number(e.target.value) })}
            />
          </label>
          <label>
            Minutos mínimos entre abastecimentos
            <input
              className="input"
              type="number"
              min={0}
              step={1}
              value={form.min_minutes_between_fuelings}
              onChange={(e) => setForm({ ...form, min_minutes_between_fuelings: Number(e.target.value) })}
            />
          </label>
        </div>
        <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 10 }}>
          Exemplo: consumo esperado 10 KM/L com 20% de tolerância gera alerta abaixo de 8 KM/L.
        </p>
        <div className="row" style={{ marginTop: 12, gap: 8, flexWrap: "wrap" }}>
          <button type="button" className="btn btn--primary" disabled={save.isPending || q.isLoading} onClick={() => save.mutate()}>
            {save.isPending ? "A guardar…" : "Guardar"}
          </button>
          <button
            type="button"
            className="btn"
            onClick={() => {
              setWipeStep("idle");
              setWipeConfirm("");
              setImportOpen(true);
            }}
          >
            Importação em massa
          </button>
        </div>
      </div>

      {importOpen
        ? createPortal(
            <div
              className="modal-backdrop"
              role="presentation"
              onMouseDown={() => {
                if (purgeExpenses.isPending || wipeStep === "confirm") return;
                setImportOpen(false);
                setWipeStep("idle");
                setWipeConfirm("");
              }}
            >
              <div className="modal modal--wide" role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
                <h3>Importação em massa</h3>
                <p style={{ color: "var(--muted)", marginTop: 0 }}>
                  Baixe a planilha modelo (CSV para Excel), preencha e importe. Combustível e centro de custo devem existir no
                  cadastro. Placa e CPF duplicados são recusados. O total da despesa é calculado por valor unitário × quantidade.
                </p>
                <div className="fleet-import-grid">
          <section className="fleet-import-block">
            <h3 style={{ display: "flex", alignItems: "center", gap: 6 }}>
              Veículos
              <InfoHint label="Colunas da planilha de veículos">
                <p>
                  Colunas: descrição, placa (opcional), ano, modelo, cor, cidade, UF, combustível, capacidade do tanque,
                  consumos (KM/L), hodômetro, centro de custo, status e observação. Status: ativo, inativo, manutenção,
                  parado, vendido, baixado, locado.
                </p>
                <p>Veículos sem placa (ex.: aguardando emplacamento) aparecem como "Veículo não identificado" nos relatórios.</p>
              </InfoHint>
            </h3>
            <div className="fleet-import-actions">
              <button
                type="button"
                className="btn"
                onClick={() =>
                  downloadBlob("modelo_importacao_veiculos.csv", buildExcelCsvBlob([VEHICLE_CSV_HEADER, VEHICLE_CSV_SAMPLE]))
                }
              >
                Baixar modelo CSV
              </button>
              {canMutate ? (
                <button
                  type="button"
                  className="btn btn--primary"
                  disabled={importVehicles.isPending}
                  onClick={() => vehicleFileRef.current?.click()}
                >
                  {importVehicles.isPending ? "A importar…" : "Importar CSV"}
                </button>
              ) : null}
              <input
                ref={vehicleFileRef}
                type="file"
                accept=".csv,text/csv"
                hidden
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  e.target.value = "";
                  if (f) importVehicles.mutate(f);
                }}
              />
            </div>
            {vehicleFailed.length > 0 ? (
              <ul className="fleet-import-errors">
                {vehicleFailed.slice(0, 12).map((x) => (
                  <li key={`${x.line}-${x.error}`}>
                    Linha {x.line}: {x.error}
                  </li>
                ))}
                {vehicleFailed.length > 12 ? <li>… e mais {vehicleFailed.length - 12} erro(s)</li> : null}
              </ul>
            ) : null}
          </section>

          <section className="fleet-import-block">
            <h3 style={{ display: "flex", alignItems: "center", gap: 6 }}>
              Motoristas
              <InfoHint label="Colunas da planilha de motoristas">
                <p>
                  Colunas: nome, CPF, RG, telefone, e-mail, CNH, categoria, validade (AAAA-MM-DD ou DD/MM/AAAA), cidade, UF,
                  usuário (login do sistema, opcional), status e observação. Status: ativo, inativo, bloqueado.
                </p>
              </InfoHint>
            </h3>
            <div className="fleet-import-actions">
              <button
                type="button"
                className="btn"
                onClick={() =>
                  downloadBlob("modelo_importacao_motoristas.csv", buildExcelCsvBlob([DRIVER_CSV_HEADER, DRIVER_CSV_SAMPLE]))
                }
              >
                Baixar modelo CSV
              </button>
              {canMutate ? (
                <button
                  type="button"
                  className="btn btn--primary"
                  disabled={importDrivers.isPending}
                  onClick={() => driverFileRef.current?.click()}
                >
                  {importDrivers.isPending ? "A importar…" : "Importar CSV"}
                </button>
              ) : null}
              <input
                ref={driverFileRef}
                type="file"
                accept=".csv,text/csv"
                hidden
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  e.target.value = "";
                  if (f) importDrivers.mutate(f);
                }}
              />
            </div>
            {driverFailed.length > 0 ? (
              <ul className="fleet-import-errors">
                {driverFailed.slice(0, 12).map((x) => (
                  <li key={`${x.line}-${x.error}`}>
                    Linha {x.line}: {x.error}
                  </li>
                ))}
                {driverFailed.length > 12 ? <li>… e mais {driverFailed.length - 12} erro(s)</li> : null}
              </ul>
            ) : null}
          </section>

          <section className="fleet-import-block">
            <h3 style={{ display: "flex", alignItems: "center", gap: 6 }}>
              Despesas
              <InfoHint label="Colunas da planilha de despesas">
                <p>
                  Colunas: lançamento (despesa ou abastecimento), descrição, placa, data, tipo de despesa (ou combustível, se
                  for abastecimento), valor unitário, quantidade, valor (calculado), KM e observação. Em abastecimento,
                  quantidade = litros e valor unitário = preço/L.
                </p>
              </InfoHint>
            </h3>
            <div className="fleet-import-actions">
              <button
                type="button"
                className="btn"
                onClick={() =>
                  downloadBlob(
                    "modelo_importacao_despesas.csv",
                    buildExcelCsvBlob([EXPENSE_CSV_HEADER, EXPENSE_CSV_SAMPLE, EXPENSE_CSV_SAMPLE_FUEL]),
                  )
                }
              >
                Baixar modelo CSV
              </button>
              {canMutate ? (
                <button
                  type="button"
                  className="btn btn--primary"
                  disabled={importExpenses.isPending}
                  onClick={() => expenseFileRef.current?.click()}
                >
                  {importExpenses.isPending ? "A importar…" : "Importar CSV"}
                </button>
              ) : null}
              <input
                ref={expenseFileRef}
                type="file"
                accept=".csv,text/csv"
                hidden
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  e.target.value = "";
                  if (f) importExpenses.mutate(f);
                }}
              />
            </div>
            {expenseFailed.length > 0 ? (
              <ul className="fleet-import-errors">
                {expenseFailed.slice(0, 12).map((x) => (
                  <li key={`${x.line}-${x.error}`}>
                    Linha {x.line}: {x.error}
                  </li>
                ))}
                {expenseFailed.length > 12 ? <li>… e mais {expenseFailed.length - 12} erro(s)</li> : null}
              </ul>
            ) : null}

            {canMutate ? (
              <div className="fleet-import-danger fleet-import-danger--compact">
                <h3 style={{ display: "flex", alignItems: "center", gap: 6 }}>
                  Zerar despesas
                  <InfoHint label="Sobre zerar despesas">
                    <p>
                      Apaga <strong>todas</strong> as despesas e abastecimentos da base. Veículos, motoristas e cadastros não
                      são afetados. Esta ação não tem volta.
                    </p>
                  </InfoHint>
                </h3>
                <label className="row" style={{ gap: 6, marginTop: 6, fontSize: 12 }}>
                  <input type="checkbox" checked={wipeBackup} onChange={(e) => setWipeBackup(e.target.checked)} />
                  Backup CSV antes de apagar
                </label>
                {wipeStep === "idle" ? (
                  <div className="fleet-import-actions" style={{ marginTop: 8 }}>
                    <button
                      type="button"
                      className="btn btn--danger btn--sm"
                      onClick={() => {
                        setWipeConfirm("");
                        setWipeStep("confirm");
                      }}
                    >
                      Apagar todas as despesas
                    </button>
                  </div>
                ) : (
                  <div style={{ marginTop: 8 }}>
                    <p style={{ margin: "0 0 6px", fontSize: 12, color: "var(--err, #c44)" }}>
                      Digite <strong>ZERAR</strong> para confirmar.
                    </p>
                    <input
                      className="input"
                      style={{ maxWidth: 160 }}
                      value={wipeConfirm}
                      onChange={(e) => setWipeConfirm(e.target.value)}
                      placeholder="ZERAR"
                      autoComplete="off"
                    />
                    <div className="fleet-import-actions" style={{ marginTop: 6 }}>
                      <button type="button" className="btn btn--sm" disabled={purgeExpenses.isPending} onClick={() => { setWipeStep("idle"); setWipeConfirm(""); }}>
                        Cancelar
                      </button>
                      <button
                        type="button"
                        className="btn btn--danger btn--sm"
                        disabled={wipeConfirm.trim().toUpperCase() !== "ZERAR" || purgeExpenses.isPending}
                        onClick={() => purgeExpenses.mutate()}
                      >
                        {purgeExpenses.isPending ? (wipeBackup ? "A gerar backup e apagar…" : "A apagar…") : "Confirmar e zerar"}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            ) : null}
          </section>
        </div>
                <div className="row" style={{ marginTop: 16, justifyContent: "flex-end" }}>
                  <button
                    type="button"
                    className="btn"
                    disabled={purgeExpenses.isPending}
                    onClick={() => {
                      setImportOpen(false);
                      setWipeStep("idle");
                      setWipeConfirm("");
                    }}
                  >
                    Fechar
                  </button>
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}
    </div>
  );
}
