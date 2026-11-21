import { repoSuggestion, type GitHubLink } from "./github";
import type { Project, WorkspaceData } from "./workspace";

export const githubLinkIdentity = (link: GitHubLink) => `${link.connectionId && link.connectionId !== "legacy" ? link.connectionId : "legacy"}:${link.id}:${link.fullName.toLowerCase()}`;

export function migrateProjectGitHub(workspace: WorkspaceData): { workspace: WorkspaceData; changed: boolean } {
  let changed = false;
  const projects = workspace.projects.map((project) => {
    const legacy = (project as Project & { github?: GitHubLink }).github;
    if (!legacy) return project;
    changed = true;
    let assigned = false;
    const services = project.services?.map((service) => {
      if (service.github) { if (githubLinkIdentity(service.github) === githubLinkIdentity(legacy)) assigned = true; return service; }
      if (repoSuggestion(service.repository)?.toLowerCase() !== legacy.fullName.toLowerCase()) return service;
      assigned = true;
      return { ...service, github: { ...legacy } };
    });
    const pending = [...(project.unassignedGitHubLinks || [])];
    if (!assigned && !pending.some((link) => githubLinkIdentity(link) === githubLinkIdentity(legacy))) pending.push({ ...legacy });
    const migrated = { ...project, ...(services ? { services } : {}), ...(pending.length ? { unassignedGitHubLinks: pending } : {}) } as Project & { github?: GitHubLink };
    delete migrated.github;
    return migrated;
  });
  return { workspace: changed ? { ...workspace, projects } : workspace, changed };
}
