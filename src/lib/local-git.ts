export type GitFile = { path: string; index: string; worktree: string; blocked: string };
export type GitStatus = { folder: string; repository: string; branch: string; head: string; version: string; files: GitFile[]; outside_staged: number; checked_at: string };
export type GitDiff = { path: string; staged: boolean; text: string; truncated: boolean; binary: boolean };
export type GitAction = "stage" | "unstage" | "commit";
export type GitPlan = { token: string; action: GitAction; branch: string; files: string[]; message?: string; author?: string; diff: string; truncated: boolean; expires_at: string };
export type GitResult = { action: GitAction; branch: string; commit?: string; status: GitStatus };
export type GitPushPlan = { token: string; repository: string; url: string; branch: string; head: string; remote_head: string; commits: { sha: string; title: string }[]; files: string[]; uncommitted: number; expires_at: string };
export type GitPushResult = { url: string; branch: string; commit: string; verified: boolean; message: string; release_id?: string; tracking_warning?: string };
export const hasStaged = (file: GitFile) => file.index !== " " && file.index !== "?";
export const hasWorking = (file: GitFile) => file.worktree !== " " || file.index === "?";
const quotePowerShell = (value: string) => `'${value.replaceAll("'", "''")}'`;
export function gitPushTerminalCommand(plan: GitPushPlan): string {
  return `git -C ${quotePowerShell(plan.repository)} push --no-verify --no-follow-tags --recurse-submodules=no --signed=false -- ${quotePowerShell(plan.url)} ${quotePowerShell(`${plan.head}:refs/heads/${plan.branch}`)}`;
}
export function gitTerminalCommand(repository: string, action: GitAction | "status", paths: string[] = [], message = "", hasHead = true): string {
  const prefix = `git -C ${quotePowerShell(repository)}`;
  if (action === "status") return `${prefix} status`;
  if (action === "commit") {
    if (!message.trim()) return "";
    const command = `${prefix} commit -m ${quotePowerShell(message)}`;
    let legacyMessage = message.replace(/(\\*)"/g, '$1$1\\"');
    if (/\s/.test(message)) legacyMessage = legacyMessage.replace(/(\\+)$/, "$1$1");
    if (legacyMessage === message) return command;
    return `if ($PSNativeCommandArgumentPassing -in @('Standard', 'Windows')) {\n  ${command}\n} else {\n  ${prefix} commit -m ${quotePowerShell(legacyMessage)}\n}`;
  }
  if (!paths.length) return "";
  const operation = action === "stage" ? "add" : hasHead ? "restore --staged" : "rm --cached --force";
  return `${prefix} --literal-pathspecs ${operation} -- ${paths.map(quotePowerShell).join(" ")}`;
}
export async function gitRequest<T>(action: string, body: unknown, signal?: AbortSignal): Promise<T> {
  const controller = new AbortController(); const abort = () => controller.abort();
  if (signal?.aborted) abort(); signal?.addEventListener("abort", abort, { once: true });
  const isPush = action.startsWith("push-");
  const timeout = setTimeout(abort, isPush ? 105000 : 25000);
  try {
    const response = await fetch(`http://127.0.0.1:47821/git/${action}`, { method: "POST", cache: "no-store", headers: { "Content-Type": "application/json", "X-GitMe-Client": "workspace-v1" }, body: JSON.stringify(body), signal: controller.signal });
    if (!response.ok) { if ([404, 405].includes(response.status)) throw new Error(isPush ? "Push controls need the updated companion. Restart it with npm.cmd run companion:dev." : "Local Git is unavailable. Restart the updated companion."); throw new Error((await response.text()).trim().slice(0, 400) || "Git operation failed."); }
    return response.json();
  } catch (error) { if (controller.signal.aborted) throw new Error(isPush ? "Push request stopped or timed out. Review the GitHub branch again before retrying; a push may have completed." : "Git request stopped or timed out. Refresh status before retrying."); if (error instanceof TypeError) throw new Error(isPush ? "Could not reach the Go companion. Review the GitHub branch before retrying a push." : "Could not reach the Go companion. Refresh status before retrying."); throw error; }
  finally { clearTimeout(timeout); signal?.removeEventListener("abort", abort); }
}
