import type { VercelDeployment, VercelLink } from "./vercel";

export type TrackedRelease = {
  id: string; project_id: string; service_id: string; repository: string; branch: string; commit: string;
  source: "push" | "observed"; remote_verified: boolean; created_at: string; target: VercelLink | null;
  state: "waiting" | "matched" | "not-observed" | "unlinked" | "link-changed" | "source-changed" | "unavailable";
  message: string;
  reading: { deployments: VercelDeployment[]; production_id: string; warnings: string[]; checked_at: string } | null;
};
export type ReleaseTracking = { history: Pick<TrackedRelease, "id" | "commit" | "branch" | "source" | "created_at">[]; release: TrackedRelease | null };
export async function trackingRequest<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const controller = new AbortController(); const abort = () => controller.abort();
  if (signal?.aborted) abort(); signal?.addEventListener("abort", abort, { once: true });
  const timer = setTimeout(abort, body === undefined ? 23000 : 105000);
  try {
    const response = await fetch(`http://127.0.0.1:47821/releases/${path}`, { method: body === undefined ? "GET" : "POST", cache: "no-store", signal: controller.signal, headers: body === undefined ? { Accept: "application/json" } : { "Content-Type": "application/json", "X-GitMe-Client": "workspace-v1" }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
    if (!response.ok) { if ([404,405].includes(response.status)) throw new Error("Tracking is unavailable. Restart the updated Go companion or refresh this service."); throw new Error((await response.text()).trim().slice(0,400) || "Could not read deployment tracking."); }
    return response.json();
  } catch (error) { if (controller.signal.aborted) throw new Error("Tracking check stopped or timed out. Refresh to retry."); if (error instanceof TypeError) throw new Error("Could not reach the Go companion."); throw error; }
  finally { clearTimeout(timer); signal?.removeEventListener("abort",abort); }
}
