export type VercelLink = { connectionId: string; projectId: string; projectName: string; teamId: string; scopeSlug: string };
export type VercelConnection = { id: string; username: string; connected: boolean; stored: boolean; checked_at: string; message: string };
export type VercelTeam = { id: string; slug: string; name: string };
export type VercelTeamPage = { teams: VercelTeam[]; pagination: { next: number | string | null } };
export type VercelProject = { id: string; name: string; accountId: string; framework: string; repository?: string };
export type VercelProjectPage = { projects: VercelProject[]; pagination: { next: number | string | null } };
export type VercelDeployment = { id: string; url: string; state: string; target: string; created_at: number; branch: string; sha: string };
export type VercelActivity = { project: VercelProject; production: VercelDeployment | null; deployments: VercelDeployment[]; warnings: string[]; checked_at: string };
export const tokenURL = "https://vercel.com/account/settings/tokens";
const id = (value: unknown): value is string => typeof value === "string" && /^[A-Za-z0-9_-]{1,80}$/.test(value);
const slug = (value: unknown): value is string => typeof value === "string" && /^[A-Za-z0-9_-]{1,100}$/.test(value);
export function validVercelLink(value: unknown): value is VercelLink {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const link = value as Record<string, unknown>;
  return Object.keys(link).length === 5 && Object.keys(link).every((key) => ["connectionId", "projectId", "projectName", "teamId", "scopeSlug"].includes(key)) && id(link.connectionId) && id(link.projectId) && link.projectId.startsWith("prj_") && slug(link.projectName) && (link.teamId === "" || id(link.teamId)) && slug(link.scopeSlug);
}
export const projectURL = (scope: string, name: string) => `https://vercel.com/${encodeURIComponent(scope)}/${encodeURIComponent(name)}`;
export function deploymentURL(value: string): string | undefined {
  try { const url = new URL(value); if (url.protocol === "https:" && !url.username && !url.password && !url.port && url.hostname.endsWith(".vercel.app") && !url.search && !url.hash && ["", "/"].includes(url.pathname)) return url.href; } catch {}
}
export async function vercelRequest<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const controller = new AbortController(); const abort = () => controller.abort();
  if (signal?.aborted) abort(); signal?.addEventListener("abort", abort, { once: true });
  const timeout = setTimeout(abort, 12000);
  try {
    const response = await fetch(`http://127.0.0.1:47821/vercel/${path}`, { method: body === undefined ? "GET" : "POST", cache: "no-store", signal: controller.signal, headers: body === undefined ? { Accept: "application/json" } : { "Content-Type": "application/json", "X-GitMe-Client": "workspace-v1" }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
    if (!response.ok) { if ([404, 405].includes(response.status)) throw new Error("Vercel integration is unavailable. Restart the updated Go companion."); throw new Error((await response.text()).trim().slice(0, 400) || `Vercel request failed (HTTP ${response.status}).`); }
    return await response.json();
  } catch (error) { if (controller.signal.aborted) throw new Error("Vercel request stopped or timed out. Retry when connected."); if (error instanceof TypeError) throw new Error("Could not reach the Go companion."); throw error; }
  finally { clearTimeout(timeout); signal?.removeEventListener("abort", abort); }
}
