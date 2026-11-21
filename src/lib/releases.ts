export type ReleaseRoute = "github" | "vercel-remote" | "vercel-local";
export type ReleaseConfig = { route: ReleaseRoute; target: "preview" | "production"; branch: string };
export type ReleaseInspection = { folder: string; folder_ready: boolean; folder_error: string; repository_folder: string; git: { found: boolean; branch: string; repository: string; detached: boolean; commit: string }; git_located: boolean; vercel_located: boolean; checked_at: string };
export const releaseRoutes: { id: ReleaseRoute; title: string; description: string; steps: string[] }[] = [
  { id: "github", title: "GitHub first", description: "Commit and push, then follow the hosting provider.", steps: ["Review changes", "Stage", "Commit", "Push", "Track deployment"] },
  { id: "vercel-remote", title: "Vercel from remote code", description: "Deploy or rebuild a commit already on GitHub.", steps: ["Choose version", "Review target", "Deploy", "Track deployment"] },
  { id: "vercel-local", title: "Local code to Vercel", description: "Upload local code without pushing to GitHub.", steps: ["Review files", "Review target", "Upload", "Track deployment"] },
];
export function validReleaseConfig(value: unknown): value is ReleaseConfig {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const c = value as Record<string, unknown>;
  return Object.keys(c).length === 3 && Object.keys(c).every((key) => ["route", "target", "branch"].includes(key)) && releaseRoutes.some((route) => route.id === c.route) && (c.target === "preview" || c.target === "production") && typeof c.branch === "string" && c.branch.length <= 100 && !/[\x00\r\n\t \\~^:?*\[]/.test(c.branch) && !c.branch.includes("..") && !c.branch.includes("@{") && !c.branch.startsWith("-");
}
export async function inspectRelease(projectId: string, serviceId: string, signal?: AbortSignal): Promise<ReleaseInspection> {
  const controller = new AbortController(); const abort = () => controller.abort();
  if (signal?.aborted) abort(); signal?.addEventListener("abort", abort, { once: true });
  const timeout = setTimeout(abort, 8000);
  try {
    const response = await fetch("http://127.0.0.1:47821/releases/inspect", { method: "POST", cache: "no-store", headers: { "Content-Type": "application/json", "X-GitMe-Client": "workspace-v1" }, body: JSON.stringify({ project_id: projectId, service_id: serviceId }), signal: controller.signal });
    if (!response.ok) { if ([404, 405].includes(response.status)) throw new Error("Release inspection is unavailable. Restart the updated companion or check that the service still exists."); throw new Error((await response.text()).trim().slice(0, 300) || "Could not inspect this service."); }
    return response.json();
  } catch (error) { if (controller.signal.aborted) throw new Error("Service inspection stopped or timed out."); if (error instanceof TypeError) throw new Error("Could not reach the Go companion."); throw error; }
  finally { clearTimeout(timeout); signal?.removeEventListener("abort", abort); }
}
