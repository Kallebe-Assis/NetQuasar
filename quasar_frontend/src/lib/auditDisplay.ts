export function prettyAuditDiff(
  beforeData: Record<string, unknown> | null | undefined,
  afterData: Record<string, unknown> | null | undefined,
): string {
  const b = beforeData ?? {};
  const a = afterData ?? {};
  const keys = Array.from(new Set([...Object.keys(b), ...Object.keys(a)])).sort();
  const rows: string[] = [];
  for (const k of keys) {
    const bv = JSON.stringify(b[k]);
    const av = JSON.stringify(a[k]);
    if (bv === av) continue;
    rows.push(`${k}: ${bv ?? "∅"} → ${av ?? "∅"}`);
  }
  if (rows.length) return rows.join("\n");
  const rawB = beforeData && Object.keys(beforeData).length > 0 ? JSON.stringify(beforeData, null, 2) : "";
  const rawA = afterData && Object.keys(afterData).length > 0 ? JSON.stringify(afterData, null, 2) : "";
  if (!rawB && !rawA) return "—";
  if (!rawB) return rawA;
  if (!rawA) return rawB;
  return `${rawB}\n\n${rawA}`;
}
