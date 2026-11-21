import type { WorkspaceData } from "./workspace";
import { validGitHubLink } from "./github";
import { validVercelLink } from "./vercel";
import { validReleaseConfig } from "./releases";
import { validSavedApiRequests } from "./api-tester";

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const strings = (value: Record<string, unknown>, keys: string[]) => keys.every((key) => typeof value[key] === "string");
const stringArray = (value: unknown) => Array.isArray(value) && value.every((item) => typeof item === "string");
function service(value: Record<string, unknown>) {
  return (value.release === undefined || validReleaseConfig(value.release)) && (value.vercel === undefined || validVercelLink(value.vercel)) && (value.github === undefined || validGitHubLink(value.github)) && strings(value, ["name", "runtime", "folder", "repository", "branch", "healthPath"])
    && ["frontend", "backend", "worker", "database", "other"].includes(String(value.role)) && stringArray(value.stack)
    && (value.port === null || (typeof value.port === "number" && Number.isInteger(value.port) && value.port > 0 && value.port <= 65535))
    && object(value.commands) && strings(value.commands, ["install", "develop", "build", "test", "start"])
    && object(value.deployment) && strings(value.deployment, ["provider", "method", "build", "start", "dockerfile", "image"])
    && object(value.environments) && ["local", "preview", "production"].every((name) => { const config = (value.environments as Record<string, unknown>)[name]; return object(config) && strings(config, ["url"]) && stringArray(config.variables); })
    && typeof value.updatedAt === "number" && value.updatedAt > 0
    && (value.detection === undefined || (object(value.detection) && strings(value.detection, ["key", "root", "scannedAt"]) && stringArray(value.detection.files)))
    && (value.commandOverrides === undefined || (stringArray(value.commandOverrides) && (value.commandOverrides as string[]).every((command) => ["install", "develop", "build", "test", "start"].includes(command))));
}
const list = (value: unknown, validate: (item: Record<string, unknown>) => boolean, identified = true) => {
  if (!Array.isArray(value) || value.length > 10000) return false;
  const ids = new Set<string>();
  return value.every((item) => {
    if (!object(item) || !validate(item)) return false;
    if (!identified) return true;
    if (typeof item.id !== "string" || !item.id || ids.has(item.id)) return false;
    ids.add(item.id); return true;
  });
};

export function isWorkspace(value: unknown): value is WorkspaceData {
  if (!object(value) || value.version !== 1) return false;
  if (value.workspaceName !== undefined && (typeof value.workspaceName !== "string" || value.workspaceName.length > 60)) return false;
  return list(value.projects, (p) => (p.apiRequests === undefined || validSavedApiRequests(p.apiRequests)) && (p.vercel === undefined || validVercelLink(p.vercel)) && (p.github === undefined || validGitHubLink(p.github)) && (p.unassignedGitHubLinks === undefined || (Array.isArray(p.unassignedGitHubLinks) && p.unassignedGitHubLinks.length <= 100 && p.unassignedGitHubLinks.every(validGitHubLink))) && strings(p, ["name", "initials", "color", "description", "stage", "branch", "folder", "url", "local", "note"]) && stringArray(p.stack) && typeof p.progress === "number" && p.progress >= 0 && p.progress <= 100 && (p.services === undefined || list(p.services, service)))
    && list(value.tasks, (t) => strings(t, ["title", "project", "priority"]) && ["Planned", "In progress", "Done"].includes(String(t.status)))
    && list(value.notes, (n) => strings(n, ["title", "body", "project"]) && ["note", "plan"].includes(String(n.kind)) && typeof n.pinned === "boolean" && typeof n.updatedAt === "number" && n.updatedAt > 0)
    && list(value.sessions, (s) => strings(s, ["project"]) && typeof s.started === "number" && s.started > 0 && typeof s.ended === "number" && s.ended >= s.started, false)
    && list(value.drafts, (d) => strings(d, ["platform", "project", "text"]))
    && object(value.document) && strings(value.document, ["title", "text"])
    && (value.focus === null || (object(value.focus) && strings(value.focus, ["project"]) && typeof value.focus.started === "number" && value.focus.started > 0 && (value.focus.elapsedMs === undefined || (typeof value.focus.elapsedMs === "number" && Number.isSafeInteger(value.focus.elapsedMs) && value.focus.elapsedMs >= 0))))
    && (value.cvDraft === undefined || (object(value.cvDraft) && strings(value.cvDraft, ["name", "role", "email", "phone", "location", "website", "profile", "experience", "education", "skills"]) && ["classic", "compact"].includes(String(value.cvDraft.template))))
    && (value.postContexts === undefined || (Array.isArray(value.postContexts) && value.postContexts.length <= 1000 && value.postContexts.every(c => object(c) && strings(c, ["projectId", "projectName", "description", "repository", "extra", "platform", "tone"]) && typeof c.savedAt === "number" && Array.isArray(c.commits) && c.commits.length <= 50 && c.commits.every((commit: unknown) => object(commit) && strings(commit, ["sha", "title", "date"])))))
    && (value.lessons === undefined || list(value.lessons, lesson => strings(lesson, ["title", "category", "body"]) && typeof lesson.updatedAt === "number" && lesson.updatedAt > 0))
    && (value.libraryItems === undefined || list(value.libraryItems, item => strings(item, ["name", "category", "content", "description"]) && ["Links", "Skills", "Notes", "Commands"].includes(String(item.folder))))
    && (value.projectDirectory === undefined || typeof value.projectDirectory === "string")
    && (value.collapsedTaskGroups === undefined || (Array.isArray(value.collapsedTaskGroups) && value.collapsedTaskGroups.every((item) => typeof item === "string")));
}

