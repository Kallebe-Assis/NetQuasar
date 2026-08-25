/** Parseia saída textual do nmap para exibição amigável. */

export type NmapPortRow = {
  port: string;
  proto: string;
  state: string;
  service: string;
};

export type ParsedNmap = {
  host: string;
  hostUp: boolean | null;
  latencySec: number | null;
  notShown: string;
  ports: NmapPortRow[];
  scannedInSec: number | null;
  summary: string;
};

const PORT_LINE = /^(\d+)\/(tcp|udp|sctp)\s+(\S+)\s+(\S*)/i;

export function parseNmapOutput(raw: string): ParsedNmap {
  const text = String(raw ?? "").trim();
  const lines = text.split(/\r?\n/);

  let host = "";
  let hostUp: boolean | null = null;
  let latencySec: number | null = null;
  let notShown = "";
  let scannedInSec: number | null = null;
  let summary = "";
  const ports: NmapPortRow[] = [];

  for (const line of lines) {
    const t = line.trim();
    if (!t) continue;

    const report = t.match(/^Nmap scan report for\s+(.+)$/i);
    if (report) {
      host = report[1].trim();
      continue;
    }

    const upLat = t.match(/^Host is up\s*\(([\d.]+)s\s+latency\)\.?$/i);
    if (upLat) {
      hostUp = true;
      latencySec = Number(upLat[1]);
      continue;
    }
    if (/^Host is up\.?$/i.test(t)) {
      hostUp = true;
      continue;
    }
    if (/Host (?:is|seems) down/i.test(t)) {
      hostUp = false;
      continue;
    }

    const ns = t.match(/^Not shown:\s*(.+)$/i);
    if (ns) {
      notShown = ns[1].trim();
      continue;
    }

    const port = t.match(PORT_LINE);
    if (port) {
      ports.push({
        port: port[1],
        proto: port[2].toLowerCase(),
        state: port[3].toLowerCase(),
        service: (port[4] || "").trim() || "—",
      });
      continue;
    }

    const done = t.match(/^Nmap done:\s*(.+)$/i);
    if (done) {
      summary = done[1].trim();
      const sec = summary.match(/scanned in\s+([\d.]+)\s*seconds?/i);
      if (sec) scannedInSec = Number(sec[1]);
    }
  }

  if (hostUp === null && /1 host up/i.test(summary)) hostUp = true;
  if (hostUp === null && /0 hosts? up/i.test(summary)) hostUp = false;

  return { host, hostUp, latencySec, notShown, ports, scannedInSec, summary };
}

export function nmapStateBadgeClass(state: string): string {
  const s = state.toLowerCase();
  if (s === "open") return "badge badge--ok";
  if (s === "closed") return "badge badge--off";
  if (s.includes("filtered")) return "badge";
  return "badge badge--off";
}

export function nmapStateLabel(state: string): string {
  const s = state.toLowerCase();
  if (s === "open") return "Aberta";
  if (s === "closed") return "Fechada";
  if (s === "filtered") return "Filtrada";
  if (s === "open|filtered") return "Aberta/filtrada";
  return state;
}
