import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useLocation } from "react-router-dom";
import { displayAlertType, displaySeverity } from "../lib/alertLabels";
import { armAlertAudioUnlock, playAlertSound } from "../lib/alertSound";
import { apiFetch } from "../lib/api";
import { useAppToast } from "../lib/appToast";
import { getAuthToken } from "../lib/auth";
import { queryKeys } from "../lib/queryKeys";
import { fetchMyPreferences } from "../lib/userPreferences";

type WatchAlert = {
  id: string;
  type: string;
  severity: string;
  message: string;
  ip?: string;
  device_name?: string;
  closed_at?: string | null;
};

function isAlertsOrMonitoringPath(pathname: string): boolean {
  return pathname === "/alerts" || pathname.startsWith("/alerts/") || pathname === "/monitoring" || pathname.startsWith("/monitoring/");
}

function fingerprint(a: WatchAlert): string {
  return `${a.severity}|${a.type}|${a.message}|${a.closed_at ?? ""}`;
}

function toastTone(sev: string): "err" | "info" {
  const s = sev.toLowerCase();
  if (s === "critical" || s === "warning") return "err";
  return "info";
}

export function AlertNotificationWatcher() {
  const location = useLocation();
  const { push } = useAppToast();
  const seenRef = useRef<Map<string, string> | null>(null);
  const pathRef = useRef(location.pathname);
  pathRef.current = location.pathname;

  useEffect(() => {
    armAlertAudioUnlock();
  }, []);

  const prefsQ = useQuery({
    queryKey: queryKeys.mePreferences,
    queryFn: fetchMyPreferences,
    enabled: !!getAuthToken(),
    staleTime: 15_000,
  });

  const monState = useQuery({
    queryKey: queryKeys.monState,
    queryFn: () => apiFetch<{ last_alerts_change_at?: string | null }>("/api/v1/monitoring/state"),
    enabled: !!getAuthToken(),
    refetchInterval: 1500,
    staleTime: 1000,
  });

  const alertsQ = useQuery({
    queryKey: ["alerts-active-notify", monState.data?.last_alerts_change_at ?? ""],
    queryFn: () => apiFetch<{ alerts: WatchAlert[] }>("/api/v1/alerts/active?limit=250"),
    enabled: !!getAuthToken(),
    refetchInterval: 12_000,
    staleTime: 2000,
  });

  const prefs = prefsQ.data;
  const toastEverywhere = prefs?.alert_toast_everywhere !== false;
  const soundOn = prefs?.alert_sound_enabled !== false;
  const soundId = prefs?.alert_sound_id ?? "builtin:alert";

  useEffect(() => {
    const list = (alertsQ.data?.alerts ?? []).filter((a) => !a.closed_at);
    if (!alertsQ.data) return;

    if (seenRef.current == null) {
      const seed = new Map<string, string>();
      for (const a of list) seed.set(a.id, fingerprint(a));
      seenRef.current = seed;
      return;
    }

    const prev = seenRef.current;
    const next = new Map<string, string>();
    const events: WatchAlert[] = [];
    for (const a of list) {
      const fp = fingerprint(a);
      next.set(a.id, fp);
      const old = prev.get(a.id);
      if (old == null || old !== fp) events.push(a);
    }
    seenRef.current = next;
    if (events.length === 0) return;

    const showToast = toastEverywhere || isAlertsOrMonitoringPath(pathRef.current);
    let played = false;
    for (const a of events.slice(0, 6)) {
      if (showToast) {
        if (a.type === "ping_unreachable") {
          push({
            tone: "err",
            text: "",
            kind: "offline",
            offlineTitle: a.device_name || "Equipamento offline",
            offlineIp: a.ip || "—",
            autoMs: 9000,
          });
        } else {
          const who = [a.device_name, a.ip].filter(Boolean).join(" · ");
          const typeLabel = displayAlertType(a.type);
          const sev = displaySeverity(a.severity);
          push({
            tone: toastTone(a.severity),
            text: `${sev} · ${typeLabel}${who ? ` — ${who}` : ""}`,
            autoMs: 9000,
          });
        }
      }
      if (soundOn && !played) {
        played = true;
        void playAlertSound(soundId);
      }
    }
  }, [alertsQ.data, push, soundId, soundOn, toastEverywhere]);

  return null;
}
