const isoTimestamp = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

export function normalizeWorkspace(value: unknown): { workspace: unknown; changed: boolean } {
  if (!value || typeof value !== "object" || Array.isArray(value)) return { workspace: value, changed: false };
  const workspace = value as Record<string, unknown>;
  if (!Array.isArray(workspace.lessons)) return { workspace: value, changed: false };
  let changed = false;
  const lessons = workspace.lessons.map((lesson: unknown) => {
    if (!lesson || typeof lesson !== "object" || Array.isArray(lesson)) return lesson;
    const record = lesson as Record<string, unknown>;
    if (typeof record.updatedAt !== "string" || !isoTimestamp.test(record.updatedAt)) return lesson;
    const updatedAt = Date.parse(record.updatedAt);
    if (!Number.isFinite(updatedAt) || updatedAt <= 0) return lesson;
    changed = true;
    return { ...record, updatedAt };
  });
  return { workspace: changed ? { ...workspace, lessons } : value, changed };
}
