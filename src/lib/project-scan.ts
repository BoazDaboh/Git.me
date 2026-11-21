import { createServiceDraft, serviceCommands, type ServiceProfile } from "./services";
import type { Project } from "./workspace";

type GitInfo = { found: boolean; branch: string; repository: string; detached: boolean };
export type DetectedService = {
  key: string; kind: "node" | "go" | "php" | "browser" | "container"; name: string; folder: string; relative_folder: string;
  role: ServiceProfile["role"]; runtime: string; stack: string[]; commands: ServiceProfile["commands"]; port: number | null;
  files: string[]; notes: string[]; git: GitInfo; dockerfile: string; package_manager?: string; checks?:{name:string;command:string;slot:string}[];
};
export type ProjectScanResult = { folder: string; scanned_at: string; directories: number; services: DetectedService[]; git: GitInfo; warnings: string[] };
const normalized = (path: string) => path.replace(/\\/g, "/").replace(/\/+$/, "").toLowerCase();

export function matchingProfile(project: Project, candidate: DetectedService, result: ProjectScanResult) {
  const profiles = project.services ?? [];
  const exact = profiles.find((profile) => profile.detection?.key === candidate.key && normalized(profile.detection.root) === normalized(result.folder));
  if (exact) return exact;
  if (result.services.filter((item) => normalized(item.folder) === normalized(candidate.folder) && item.kind === candidate.kind).length > 1) return undefined;
  return profiles.find((profile) => normalized(profile.folder) === normalized(candidate.folder) && profile.role === candidate.role && (candidate.kind === "node" ? profile.stack.includes("Node.js") || /^node/i.test(profile.runtime) : candidate.kind === "go" ? profile.stack.includes("Go") : candidate.kind === "php" ? profile.stack.includes("PHP") : candidate.kind === "browser" ? profile.runtime === "Browser" : profile.runtime === "Docker"));
}

export function scannedProfile(project: Project, candidate: DetectedService, result: ProjectScanResult, existing?: ServiceProfile, replaceCommands = false): ServiceProfile {
  const draft = createServiceDraft(project, candidate.role);
  const detected: ServiceProfile = {
    ...draft, name: candidate.name.slice(0, 80), folder: candidate.folder, runtime: candidate.runtime, stack: candidate.stack.slice(0, 20),
    repository: candidate.git.repository, branch: candidate.git.branch, port: candidate.port, healthPath: "",
    commands: candidate.commands, commandTemplate: "custom", commandOverrides: serviceCommands.map(({ value }) => value),
    deployment: { provider: "", method: candidate.dockerfile ? "container" : candidate.stack.includes("Next.js") ? "managed" : candidate.kind === "browser" ? "static" : "native", build: candidate.dockerfile ? "" : candidate.commands.build, start: candidate.dockerfile ? "" : candidate.commands.start, dockerfile: candidate.dockerfile, image: "" },
    environments: { local: { url: candidate.port ? `http://127.0.0.1:${candidate.port}` : "", variables: [] }, preview: { url: "", variables: [] }, production: { url: "", variables: [] } },
    detection: { key: candidate.key, root: result.folder, scannedAt: result.scanned_at, files: candidate.files },
  };
  if (!existing) return detected;
  return { ...detected, ...existing, stack: [...new Set([...existing.stack, ...candidate.stack])].slice(0, 20),
    folder: existing.folder || detected.folder, runtime: existing.runtime || detected.runtime,
    repository: existing.repository || detected.repository, branch: existing.branch || detected.branch,
    commands: replaceCommands ? detected.commands : existing.commands,
    commandTemplate: "custom", commandOverrides: serviceCommands.map(({ value }) => value), detection: detected.detection,
  };
}

export async function scanProject(projectId: string, signal: AbortSignal): Promise<ProjectScanResult> {
  const response = await fetch("http://127.0.0.1:47821/projects/scan", { method: "POST", cache: "no-store", signal,
    headers: { "Content-Type": "application/json", "X-GitMe-Client": "workspace-v1" }, body: JSON.stringify({ project_id: projectId }) });
  if (!response.ok) {
    if (response.status === 404 || response.status === 405) throw new Error("The project or scan endpoint was not found. Check the saved project and restart the updated Go companion.");
    const message = (await response.text()).trim().slice(0, 300);
    throw new Error(message || `Scan failed (HTTP ${response.status}).`);
  }
  const result = await response.json();
  if (!result || typeof result.folder !== "string" || typeof result.scanned_at !== "string" || !Array.isArray(result.services) || !Array.isArray(result.warnings) || !result.git || result.services.length > 48) throw new Error("Unexpected scan response. Restart the updated Go companion.");
  return result;
}
