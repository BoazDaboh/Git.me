export const companionUrl = "http://127.0.0.1:47821/health";
export const companionPollMs = 10000;
export const companionTimeoutMs = 2000;

export type CompanionConnection = {
  state: "checking" | "connected" | "offline" | "mismatch";
  checking: boolean;
  checkedAt: number | null;
  version: string | null;
  latencyMs: number | null;
  startedAt: string | null;
  lastSleepMs?: number;
  lastConnectedAt?: number;
  message: string;
};

export const initialConnection: CompanionConnection = { state: "checking", checking: true, checkedAt: null, version: null, latencyMs: null, startedAt: null, message: "Checking the local companion." };

export async function fetchCompanionHealth(signal: AbortSignal): Promise<CompanionConnection> {
  const started = performance.now();
  const empty = { checking: false, checkedAt: null, version: null, latencyMs: null, startedAt: null };
  try {
    const response = await fetch(companionUrl, { signal, cache: "no-store", headers: { Accept: "application/json" } });
    if (!response.ok) return { ...empty, state: "offline", checkedAt: Date.now(), message: `The companion returned HTTP ${response.status}. Check its terminal.` };
    let body;
    try { body = await response.json(); }
    catch { return { ...empty, state: "mismatch", checkedAt: Date.now(), message: "A service answered, but its response is not the Git/Me health format." }; }
    if (body?.app !== "gitme-companion" || body.status !== "ok" || body.protocol !== 1 || typeof body.version !== "string" || !body.version || typeof body.started_at !== "string" || !Number.isFinite(Date.parse(body.started_at)) || !Array.isArray(body.capabilities) || !body.capabilities.includes("health")) {
      return { ...empty, state: "mismatch", checkedAt: Date.now(), message: "A different or incompatible service is using the companion address." };
    }
    return { state: "connected", checking: false, checkedAt: Date.now(), version: body.version, startedAt: body.started_at, lastSleepMs: typeof body.last_sleep_ms === "number" ? body.last_sleep_ms : 0, latencyMs: Math.max(0, Math.round(performance.now() - started)), message: "The local Git/Me companion responded successfully." };
  } catch {
    return { ...empty, state: "offline", checkedAt: Date.now(), message: signal.aborted ? "The connection check timed out. Check the companion terminal and retry." : "The companion could not be reached. Start it in a separate terminal, then check again." };
  }
}
