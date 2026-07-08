import { useQuery } from "@tanstack/react-query";
import type { ChangeEvent, Dispatch, ReactNode, RefObject, SetStateAction } from "react";
import { apiFetch } from "../../lib/api";

const TELEMETRY_MODES = [
  { value: "SNMP", label: "SNMP (padrão)" },
  { value: "telnet", label: "Telnet" },
  { value: "ssh", label: "SSH" },
] as const;

export type DeviceMonitoringForm = {
  category?: string;
  network_status?: string;
  ping_enabled?: boolean;
  telemetry_enabled?: boolean;
  bng_enabled?: boolean;
  telemetry_mode?: string | null;
  snmp_community?: string | null;
  mib_folder_path?: string | null;
  telemetry_oid_strategy?: "default" | "manual" | null;
  telemetry_oid_overrides?: {
    cpu_oid?: string;
    cpu_available_oid?: string;
    memory_used_oid?: string;
    memory_size_oid?: string;
    temp_oid?: string;
    uptime_oid?: string;
  } | null;
  mikrotik_telnet_profile_id?: string | null;
  switch_telnet_profile_id?: string | null;
  telnet_user?: string | null;
  telnet_enable?: string | null;
  ssh_user?: string | null;
  telnet_password_configured?: boolean;
  ssh_password_configured?: boolean;
};

function PanelSwitch({
  id,
  label,
  checked,
  disabled,
  onChange,
}: {
  id: string;
  label: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (next: boolean) => void;
}) {
  return (
    <label className="toggle" htmlFor={id}>
      <span className="toggle__track">
        <input
          id={id}
          type="checkbox"
          role="switch"
          className="toggle__input"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span className="toggle__thumb" aria-hidden />
      </span>
      <span className="toggle__label">{label}</span>
    </label>
  );
}

export function normalizeTelemetryMode(raw: string | null | undefined): "SNMP" | "telnet" | "ssh" {
  if (!raw?.trim()) return "SNMP";
  const t = raw.trim();
  if (/^snmp$/i.test(t)) return "SNMP";
  if (t.toLowerCase() === "telnet") return "telnet";
  if (t.toLowerCase() === "ssh") return "ssh";
  return "SNMP";
}

function networkIsBridge(ns: string | undefined | null): boolean {
  return String(ns ?? "").trim() === "Bridge";
}

type Props = {
  form: Partial<DeviceMonitoringForm>;
  setForm: Dispatch<SetStateAction<Partial<DeviceMonitoringForm>>>;
  telnetPassword: string;
  setTelnetPassword: (v: string) => void;
  sshPassword: string;
  setSshPassword: (v: string) => void;
  mibFolderPickerRef: RefObject<HTMLInputElement>;
  mibBrowseNote: string | null;
  setMibBrowseNote: (v: string | null) => void;
  onMibFolderPicked: (ev: ChangeEvent<HTMLInputElement>) => void;
  onOpenMibFolderPicker: () => void;
  footer?: ReactNode;
};

export function DeviceEditMonitoramentoTab({
  form,
  setForm,
  telnetPassword,
  setTelnetPassword,
  sshPassword,
  setSshPassword,
  mibFolderPickerRef,
  mibBrowseNote,
  setMibBrowseNote,
  onMibFolderPicked,
  onOpenMibFolderPicker,
  footer,
}: Props) {
  const formIsBridge = networkIsBridge(form.network_status);
  const formIsOutros = (form.category ?? "").trim() === "Outros";
  const formIsMikrotik = (form.category ?? "").trim() === "Mikrotik";
  const formIsSwitch = (form.category ?? "").trim() === "Switch";
  const formTelemetryOIDStrategy = (form.telemetry_oid_strategy as "default" | "manual" | null) ?? "default";
  const telemetryMode = normalizeTelemetryMode(form.telemetry_mode ?? "SNMP");

  const mikrotikProfiles = useQuery({
    queryKey: ["mikrotik-telnet-profiles"],
    queryFn: () => apiFetch<{ profiles: Array<{ id: string; name: string }> }>("/api/v1/settings/mikrotik-telnet-profiles"),
    enabled: formIsMikrotik,
  });
  const switchProfiles = useQuery({
    queryKey: ["switch-telnet-profiles"],
    queryFn: () => apiFetch<{ profiles: Array<{ id: string; name: string }> }>("/api/v1/settings/switch-telnet-profiles"),
    enabled: formIsSwitch,
  });

  return (
    <>
      <p style={{ color: "var(--muted)", fontSize: 12, marginTop: 0 }}>
        Com telemetria ativa, o ping precisa estar ligado. Credenciais em branco usam os padrões globais em{" "}
        <strong>Definições → Ligações</strong>. Em modo <strong>Bridge</strong>, ping e telemetria ficam desativados.
      </p>

      <div className="device-form-grid">
        <div className="toggle-row field--full">
          <PanelSwitch
            id="device-ping-mon"
            label="Monitorar com ping"
            checked={formIsBridge ? false : !!form.ping_enabled}
            disabled={formIsBridge}
            onChange={(pingOn) =>
              setForm((f) => ({
                ...f,
                ping_enabled: pingOn,
                ...(pingOn ? {} : { telemetry_enabled: false }),
              }))
            }
          />
          <PanelSwitch
            id="device-tel-mon"
            label="Telemetria"
            checked={formIsBridge ? false : !!form.telemetry_enabled}
            disabled={formIsBridge}
            onChange={(telOn) =>
              setForm((f) => ({
                ...f,
                telemetry_enabled: telOn,
                ping_enabled: telOn ? true : f.ping_enabled,
                telemetry_mode: telOn ? normalizeTelemetryMode(f.telemetry_mode ?? "SNMP") : f.telemetry_mode,
              }))
            }
          />
          <PanelSwitch
            id="device-bng-mon"
            label="BNG"
            checked={!!form.bng_enabled}
            onChange={(bngOn) => setForm((f) => ({ ...f, bng_enabled: bngOn }))}
          />
        </div>

        {formIsBridge && (
          <p style={{ gridColumn: "1 / -1", fontSize: 12, color: "var(--muted)", margin: "-0.25rem 0 0.5rem" }}>
            Estado Bridge: ping e telemetria não estão disponíveis.
          </p>
        )}

        {!formIsBridge && (
          <>
            <div className="field">
              <label>Modo de telemetria</label>
              <select
                className="select"
                style={{ width: "100%" }}
                value={telemetryMode}
                disabled={!form.telemetry_enabled}
                onChange={(e) =>
                  setForm({
                    ...form,
                    telemetry_mode: e.target.value as "SNMP" | "telnet" | "ssh",
                  })
                }
              >
                {TELEMETRY_MODES.map((m) => (
                  <option key={m.value} value={m.value}>
                    {m.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="field">
              <label>SNMP community (opcional)</label>
              <input
                className="input mono"
                style={{ width: "100%" }}
                disabled={!form.telemetry_enabled}
                placeholder="Usa padrão global se vazio"
                value={form.snmp_community ?? ""}
                onChange={(e) => setForm({ ...form, snmp_community: e.target.value })}
              />
            </div>

            {formIsMikrotik && (
              <div className="field field--full">
                <label>Perfil Telnet MikroTik</label>
                <select
                  className="select"
                  style={{ width: "100%" }}
                  value={form.mikrotik_telnet_profile_id ?? ""}
                  onChange={(e) =>
                    setForm({ ...form, mikrotik_telnet_profile_id: e.target.value.trim() === "" ? null : e.target.value })
                  }
                >
                  <option value="">Padrão do sistema</option>
                  {(mikrotikProfiles.data?.profiles ?? []).map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </div>
            )}

            {formIsSwitch && (
              <div className="field field--full">
                <label>Perfil Telnet Switch</label>
                <select
                  className="select"
                  style={{ width: "100%" }}
                  value={form.switch_telnet_profile_id ?? ""}
                  onChange={(e) =>
                    setForm({ ...form, switch_telnet_profile_id: e.target.value.trim() === "" ? null : e.target.value })
                  }
                >
                  <option value="">Padrão do sistema</option>
                  {(switchProfiles.data?.profiles ?? []).map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </div>
            )}

            <div className="field field--full" style={{ marginTop: 4 }}>
              <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>Credenciais Telnet (equipamento)</h4>
            </div>
            <div className="field">
              <label>Utilizador Telnet</label>
              <input
                className="input mono"
                style={{ width: "100%" }}
                placeholder="Opcional — substitui o padrão global"
                value={form.telnet_user ?? ""}
                onChange={(e) => setForm({ ...form, telnet_user: e.target.value })}
              />
            </div>
            <div className="field">
              <label>Palavra-passe Telnet</label>
              <input
                type="password"
                className="input mono"
                style={{ width: "100%" }}
                placeholder={form.telnet_password_configured ? "•••••• (definida — deixe vazio para manter)" : "Opcional"}
                value={telnetPassword}
                onChange={(e) => setTelnetPassword(e.target.value)}
                autoComplete="new-password"
              />
            </div>
            <div className="field field--full">
              <label>Enable / privilégio (Telnet)</label>
              <input
                type="password"
                className="input mono"
                style={{ width: "100%" }}
                placeholder="Opcional — Cisco enable, etc."
                value={form.telnet_enable ?? ""}
                onChange={(e) => setForm({ ...form, telnet_enable: e.target.value })}
                autoComplete="new-password"
              />
            </div>

            <div className="field field--full" style={{ marginTop: 4 }}>
              <h4 style={{ margin: "0 0 8px", fontSize: 13 }}>Credenciais SSH (equipamento)</h4>
            </div>
            <div className="field">
              <label>Utilizador SSH</label>
              <input
                className="input mono"
                style={{ width: "100%" }}
                placeholder="Opcional — substitui o padrão global"
                value={form.ssh_user ?? ""}
                onChange={(e) => setForm({ ...form, ssh_user: e.target.value })}
              />
            </div>
            <div className="field">
              <label>Palavra-passe SSH</label>
              <input
                type="password"
                className="input mono"
                style={{ width: "100%" }}
                placeholder={form.ssh_password_configured ? "•••••• (definida — deixe vazio para manter)" : "Opcional"}
                value={sshPassword}
                onChange={(e) => setSshPassword(e.target.value)}
                autoComplete="new-password"
              />
            </div>

            {form.telemetry_enabled && formIsOutros && telemetryMode === "SNMP" && (
              <>
                <div className="field field--full">
                  <label>OIDs de telemetria para categoria Outros</label>
                  <select
                    className="select"
                    style={{ width: "100%" }}
                    value={formTelemetryOIDStrategy}
                    onChange={(e) =>
                      setForm((f) => ({
                        ...f,
                        telemetry_oid_strategy: e.target.value as "default" | "manual",
                        telemetry_oid_overrides: f.telemetry_oid_overrides ?? {},
                      }))
                    }
                  >
                    <option value="default">Usar OIDs padrão do sistema</option>
                    <option value="manual">Inserir OIDs manualmente neste equipamento</option>
                  </select>
                </div>
                {formTelemetryOIDStrategy === "manual" && (
                  <>
                    <div className="field">
                      <label>CPU (uso)</label>
                      <input
                        className="input mono"
                        style={{ width: "100%" }}
                        value={form.telemetry_oid_overrides?.cpu_oid ?? ""}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), cpu_oid: e.target.value },
                          }))
                        }
                      />
                    </div>
                    <div className="field">
                      <label>CPU disponível (idle)</label>
                      <input
                        className="input mono"
                        style={{ width: "100%" }}
                        value={form.telemetry_oid_overrides?.cpu_available_oid ?? ""}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), cpu_available_oid: e.target.value },
                          }))
                        }
                      />
                    </div>
                    <div className="field">
                      <label>Memória usada</label>
                      <input
                        className="input mono"
                        style={{ width: "100%" }}
                        value={form.telemetry_oid_overrides?.memory_used_oid ?? ""}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), memory_used_oid: e.target.value },
                          }))
                        }
                      />
                    </div>
                    <div className="field">
                      <label>Memória total</label>
                      <input
                        className="input mono"
                        style={{ width: "100%" }}
                        value={form.telemetry_oid_overrides?.memory_size_oid ?? ""}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), memory_size_oid: e.target.value },
                          }))
                        }
                      />
                    </div>
                    <div className="field">
                      <label>Temperatura</label>
                      <input
                        className="input mono"
                        style={{ width: "100%" }}
                        value={form.telemetry_oid_overrides?.temp_oid ?? ""}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), temp_oid: e.target.value },
                          }))
                        }
                      />
                    </div>
                    <div className="field">
                      <label>Uptime</label>
                      <input
                        className="input mono"
                        style={{ width: "100%" }}
                        value={form.telemetry_oid_overrides?.uptime_oid ?? ""}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            telemetry_oid_overrides: { ...(f.telemetry_oid_overrides ?? {}), uptime_oid: e.target.value },
                          }))
                        }
                      />
                    </div>
                  </>
                )}
              </>
            )}

            {form.telemetry_enabled && (
              <div className="field field--full">
                <label>Pasta MIBs (.txt/.csv) para discovery — caminho na máquina do servidor ou relativo ao backend (opcional)</label>
                <div className="row" style={{ gap: 8, alignItems: "stretch", flexWrap: "wrap" }}>
                  <input
                    className="input mono"
                    style={{ flex: "1 1 200px", minWidth: 0 }}
                    title="Ex.: data/mibs/marc ou caminho absoluto no servidor onde corre o backend"
                    value={form.mib_folder_path ?? ""}
                    onChange={(e) => {
                      setMibBrowseNote(null);
                      setForm({ ...form, mib_folder_path: e.target.value });
                    }}
                  />
                  <input
                    ref={mibFolderPickerRef}
                    type="file"
                    multiple
                    style={{ display: "none" }}
                    {...({ webkitdirectory: "", directory: "" } as Record<string, string>)}
                    onChange={onMibFolderPicked}
                  />
                  <button type="button" className="btn" style={{ flexShrink: 0 }} onClick={onOpenMibFolderPicker}>
                    Procurar…
                  </button>
                </div>
                {mibBrowseNote && (
                  <p style={{ fontSize: 11, color: "var(--muted)", margin: "6px 0 0", lineHeight: 1.35 }}>
                    {mibBrowseNote}
                  </p>
                )}
              </div>
            )}
          </>
        )}
      </div>

      {footer}
    </>
  );
}
