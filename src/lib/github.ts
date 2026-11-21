export type GitHubLink = { id: number; fullName: string; connectionId?: string };
export type GitHubSavedConnection = GitHubConnection & { id: string; legacy?: boolean };
export type GitHubConnection = { connected: boolean; stored: boolean; account?: { login: string; name: string; id: number }; checked_at: string; message: string };
export type GitHubRepo = { id: number; full_name: string; private: boolean; default_branch: string; pushed_at: string; archived: boolean };
export type GitHubPage = { repositories: GitHubRepo[]; next: boolean; page: number };
export type GitHubActivity = { repository: GitHubRepo; branches: { name: string; protected: boolean }[]; commits: { sha: string; title: string; author: string; date: string }[]; pulls: { number: number; title: string; draft: boolean; user: { login: string }; updated_at: string }[]; warnings: string[]; checked_at: string };
export const tokenURL = "https://github.com/settings/personal-access-tokens/new?name=GitMe&description=Read-only+project+dashboard&contents=read&pull_requests=read&expires_in=90";
export function validRepoName(value: unknown): value is string { return typeof value === "string" && /^[A-Za-z0-9_.-]{1,100}\/[A-Za-z0-9_.-]{1,100}$/.test(value) && value.split("/").every((part) => part !== "." && part !== ".."); }
export function validGitHubLink(value: unknown): value is GitHubLink { if (!value || typeof value !== "object" || Array.isArray(value)) return false; const link = value as Record<string, unknown>; return Object.keys(link).every((key) => ["id", "fullName", "connectionId"].includes(key)) && (link.connectionId === undefined || (typeof link.connectionId === "string" && (link.connectionId === "legacy" || (/^[1-9][0-9]{0,15}$/.test(link.connectionId) && Number.isSafeInteger(Number(link.connectionId)))))) && typeof link.id === "number" && Number.isSafeInteger(link.id) && link.id > 0 && validRepoName(link.fullName); }
export function repoSuggestion(value: string): string | null {
  let name = "";
  if (value.startsWith("git@github.com:")) name = value.slice("git@github.com:".length);
  else { try { const url = new URL(value); if (url.hostname !== "github.com" || url.username || url.password) return null; name = url.pathname.replace(/^\//, ""); } catch { return null; } }
  name = name.replace(/\/$/, "").replace(/\.git$/, ""); return validRepoName(name) ? name : null;
}
export const repoURL = (name: string) => `https://github.com/${name.split("/").map(encodeURIComponent).join("/")}`;
export async function githubRequest<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const controller = new AbortController(); const abort = () => controller.abort();
  if (signal?.aborted) abort(); signal?.addEventListener("abort", abort, { once: true });
  const timeout = setTimeout(abort, 12000);
  try {
    const response = await fetch(`http://127.0.0.1:47821/github/${path}`, { method: body === undefined ? "GET" : "POST", cache: "no-store", signal: controller.signal, headers: body === undefined ? { Accept: "application/json" } : { "Content-Type": "application/json", "X-GitMe-Client": "workspace-v1" }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
    if (!response.ok) { if (response.status === 404 || response.status === 405) throw new Error("GitHub integration is unavailable. Restart the updated Go companion."); throw new Error((await response.text()).trim().slice(0, 400) || `GitHub request failed (HTTP ${response.status}).`); }
    return await response.json();
  } catch (error) { if (controller.signal.aborted) throw new Error("GitHub request stopped or timed out. Retry when connected."); if (error instanceof TypeError) throw new Error("Could not reach the Go companion."); throw error; }
  finally { clearTimeout(timeout); signal?.removeEventListener("abort", abort); }
}
