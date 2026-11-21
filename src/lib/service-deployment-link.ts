import type { WorkspaceData } from "./workspace";
import type { VercelLink, VercelProject } from "./vercel";
import type { ServiceProfile } from "./services";

export function serviceRepository(profile: ServiceProfile): string {
  let repository = "";
  try {
    const url = new URL(profile.repository);
    if (url.protocol === "https:" && url.host === "github.com" && !url.username && !url.password && !url.search && !url.hash) repository = url.pathname.replace(/^\//, "").replace(/\/$/, "").replace(/\.git$/, "");
  } catch {}
  const linked = profile.github?.fullName || "";
  if (!profile.repository) repository = linked;
  if (!/^[A-Za-z0-9_.-]{1,100}\/[A-Za-z0-9_.-]{1,100}$/.test(repository) || linked && linked.toLowerCase() !== repository.toLowerCase()) return "";
  return repository;
}
export function sameDeploymentLink(a: VercelLink | undefined, b: VercelLink): boolean {
  return !!a && (["connectionId","projectId","projectName","teamId","scopeSlug"] as const).every((key) => a[key] === b[key]);
}
export function adoptSavedDeployment(data: WorkspaceData, projectId: string, serviceId: string, expected: VercelLink, repository: string, verified: VercelProject): WorkspaceData {
  const project = data.projects.find((item) => item.id === projectId);
  const service = project?.services?.find((item) => item.id === serviceId);
  if (!project || !service || service.vercel || !sameDeploymentLink(project.vercel,expected) || !repository || serviceRepository(service).toLowerCase() !== repository.toLowerCase() || verified.id !== expected.projectId || verified.repository?.toLowerCase() !== repository.toLowerCase()) throw new Error("Service, source or saved deployment changed. Review the connection again.");
  const link = {...expected,projectName:verified.name};
  return {...data,projects:data.projects.map((item) => item.id !== projectId ? item : {...item,vercel:undefined,services:item.services?.map((profile) => profile.id !== serviceId ? profile : {...profile,vercel:link})})};
}
