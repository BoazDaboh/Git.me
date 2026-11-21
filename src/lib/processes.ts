export type Run = { project_id: string; service_id: string; name: string; folder: string; command: string; port: number | null; local_url: string; health_path: string; run_id: string; state: "not_managed" | "running" | "stopping" | "stopped" | "failed" | "exited"; pid: number; started_at: string; ended_at: string | null; exit_code: number | null; error: string; logs?: string; truncated: boolean };
export type LaunchPlan = { project_id: string; service_id: string; name: string; folder: string; command: string; port: number | null; action: "start" | "restart"; token: string; expires_at: string };
export type EndpointHealth = { url: string; status: number; responding: boolean; duration_ms: number; message: string };
const base = "http://127.0.0.1:47821";
export const running = (run: Run | null) => run?.state === "running" || run?.state === "stopping";

export async function processRequest<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const controller = new AbortController();
  const abort = () => controller.abort();
  if (signal?.aborted) abort();
  signal?.addEventListener("abort", abort, { once: true });
  const timeout = setTimeout(abort, 8000);
  try {
    const response = await fetch(`${base}${path}`, { method: body === undefined ? "GET" : "POST", cache: "no-store", signal: controller.signal,
      headers: body === undefined ? { Accept: "application/json" } : { "Content-Type": "application/json", "X-GitMe-Client": "workspace-v1" }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
    if (!response.ok) { if (response.status === 404 || response.status === 405) throw new Error("Process controls are unavailable. Restart the updated Go companion."); throw new Error((await response.text()).trim().slice(0, 500) || `Request failed (HTTP ${response.status}).`); }
    return await response.json();
  } catch (error) {
    if (controller.signal.aborted) throw new Error("Request stopped or timed out. Refresh process status before retrying.");
    if (error instanceof TypeError) throw new Error("Could not reach Go. Start the companion, then refresh process status.");
    throw error;
  } finally { clearTimeout(timeout); signal?.removeEventListener("abort", abort); }
}
