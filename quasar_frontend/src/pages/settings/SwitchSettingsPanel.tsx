import { useState } from "react";
import { MikrotikCollectionPanel } from "./MikrotikCollectionPanel";
import { MikrotikTelnetProfilesPanel } from "./MikrotikTelnetProfilesPanel";

export function SwitchSettingsPanel() {
  const [sub, setSub] = useState<"snmp" | "telnet">("snmp");
  return (
    <div style={{ marginTop: 8 }}>
      <div className="tabs" style={{ marginBottom: 12 }}>
        <button type="button" className={sub === "snmp" ? "active" : ""} onClick={() => setSub("snmp")}>
          Coleta SNMP
        </button>
        <button type="button" className={sub === "telnet" ? "active" : ""} onClick={() => setSub("telnet")}>
          Perfis Telnet
        </button>
      </div>
      {sub === "snmp" ? (
        <MikrotikCollectionPanel
          embedded
          apiPath="/api/v1/settings/switch-collection"
          queryKey="switch-collection"
          saveSuccessMessage="Perfil Switch guardado."
          loadingLabel="A carregar perfil Switch…"
        />
      ) : (
        <MikrotikTelnetProfilesPanel
          apiBase="/api/v1/settings/switch-telnet-profiles"
          queryKey="switch-telnet-profiles"
        />
      )}
    </div>
  );
}
