export type MergeResult = { ok: true; value: unknown } | { ok: false };
const equal = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
const record = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === "object" && !Array.isArray(v);
const identified = (a: unknown[]) => a.every(x => record(x) && typeof x.id === "string") && new Set(a.map(x => (x as { id: string }).id)).size === a.length;

export function mergeChanges(base: unknown, local: unknown, remote: unknown): MergeResult {
  if (equal(local, remote)) return { ok: true, value: local };
  if (equal(local, base)) return { ok: true, value: remote };
  if (equal(remote, base)) return { ok: true, value: local };
  if (Array.isArray(base) && Array.isArray(local) && Array.isArray(remote) && identified(base) && identified(local) && identified(remote)) {
    const asMap = (v: unknown[]) => new Map(v.map(item => [(item as { id: string }).id, item]));
    const b = asMap(base), l = asMap(local), r = asMap(remote);
    const ids = [...new Set([...r.keys(), ...l.keys()])], out: unknown[] = [];
    for (const id of ids) {
      const result = mergeChanges(b.get(id), l.get(id), r.get(id));
      if (!result.ok) return result;
      if (result.value !== undefined) out.push(result.value);
    }
    return { ok: true, value: out };
  }
  if (record(base) && record(local) && record(remote)) {
    const out: Record<string, unknown> = {};
    for (const key of new Set([...Object.keys(base), ...Object.keys(local), ...Object.keys(remote)])) {
      if (key === "updatedAt" && typeof base[key] === "number" && typeof local[key] === "number" && typeof remote[key] === "number") { out[key] = Math.max(local[key], remote[key]); continue; }
      const result = mergeChanges(base[key], local[key], remote[key]);
      if (!result.ok) return result;
      if (result.value !== undefined) out[key] = result.value;
    }
    return { ok: true, value: out };
  }
  return { ok: false };
}
