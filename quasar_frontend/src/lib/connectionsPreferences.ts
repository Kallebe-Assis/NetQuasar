export type ConnectionsViewPrefs = {
  pageSize: number;
  sortDir: "asc" | "desc";
  showSecondaryInfo: boolean;
  hiddenColumns: string[];
};

const DEFAULT_PREFS: ConnectionsViewPrefs = {
  pageSize: 50,
  sortDir: "asc",
  showSecondaryInfo: true,
  hiddenColumns: [],
};

const PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 200, 500, 1000] as const;

export { PAGE_SIZE_OPTIONS };

function storageKey(tab: string): string {
  const uid = localStorage.getItem("netquasar_user_label") ?? "default";
  return `netquasar:connections:prefs:${uid}:${tab}`;
}

export function loadConnectionsPrefs(tab: string, defaults?: Partial<ConnectionsViewPrefs>): ConnectionsViewPrefs {
  try {
    const raw = localStorage.getItem(storageKey(tab));
    if (!raw) return { ...DEFAULT_PREFS, ...defaults };
    const parsed = JSON.parse(raw) as Partial<ConnectionsViewPrefs>;
    return {
      pageSize: PAGE_SIZE_OPTIONS.includes(parsed.pageSize as (typeof PAGE_SIZE_OPTIONS)[number])
        ? (parsed.pageSize as number)
        : (defaults?.pageSize ?? DEFAULT_PREFS.pageSize),
      sortDir: parsed.sortDir === "desc" ? "desc" : "asc",
      showSecondaryInfo: parsed.showSecondaryInfo !== false,
      hiddenColumns: Array.isArray(parsed.hiddenColumns) ? parsed.hiddenColumns.map(String) : [],
    };
  } catch {
    return { ...DEFAULT_PREFS, ...defaults };
  }
}

export function saveConnectionsPrefs(tab: string, prefs: ConnectionsViewPrefs): void {
  try {
    localStorage.setItem(storageKey(tab), JSON.stringify(prefs));
  } catch {
    /* ignore */
  }
}

export function resetConnectionsPrefs(tab: string): ConnectionsViewPrefs {
  try {
    localStorage.removeItem(storageKey(tab));
  } catch {
    /* ignore */
  }
  return { ...DEFAULT_PREFS };
}
