import type { Project } from "./workspace";
import type { GitHubLink } from "./github";
import type { VercelLink } from "./vercel";
import type { ReleaseConfig } from "./releases";
import { serviceStackForRole, suggestServiceCommands, type CommandTemplate } from "./service-commands";

export type ServiceRole = "frontend" | "backend" | "worker" | "database" | "other";
export type ServiceScope = "frontend" | "backend" | "other";
export type ServiceEnvironmentName = "local" | "preview" | "production";
export type DeploymentMethod = "managed" | "native" | "container" | "functions" | "static";
export type ServiceCommand = "install" | "develop" | "build" | "test" | "start";
export type ServiceProfile = {
  id: string;
  name: string;
  role: ServiceRole;
  stack: string[];
  runtime: string;
  folder: string;
  repository: string;
  branch: string;
  port: number | null;
  healthPath: string;
  commands: Record<ServiceCommand, string>;
  commandTemplate?: CommandTemplate;
  commandOverrides?: ServiceCommand[];
  detection?: { key: string; root: string; scannedAt: string; files: string[] };
  github?: GitHubLink;
  vercel?: VercelLink;
  release?: ReleaseConfig;
  deployment: { provider: string; method: DeploymentMethod; build: string; start: string; dockerfile: string; image: string };
  environments: Record<ServiceEnvironmentName, { url: string; variables: string[] }>;
  updatedAt: number;
};

export const serviceRoles: { value: ServiceRole; label: string }[] = [
  { value: "frontend", label: "Frontend" }, { value: "backend", label: "Backend API" },
  { value: "worker", label: "Worker" }, { value: "database", label: "Database" }, { value: "other", label: "Other" },
];
export const deploymentMethods: { value: DeploymentMethod; label: string }[] = [
  { value: "managed", label: "Managed framework" }, { value: "native", label: "Native process" },
  { value: "container", label: "Container" }, { value: "functions", label: "Functions" }, { value: "static", label: "Static files" },
];
export const serviceCommands: { value: ServiceCommand; label: string }[] = [
  { value: "install", label: "Install" }, { value: "develop", label: "Development" }, { value: "build", label: "Build" }, { value: "test", label: "Test / check" }, { value: "start", label: "Start built app" },
];
export const environmentNames: { value: ServiceEnvironmentName; label: string }[] = [
  { value: "local", label: "Local" }, { value: "preview", label: "Preview" }, { value: "production", label: "Production" },
];
export const serviceScope = (role: ServiceRole): ServiceScope => role === "frontend" || role === "backend" ? role : "other";

export function createServiceDraft(project: Project, role: ServiceRole): ServiceProfile {
  const isNext = role === "frontend" && project.stack.includes("Next.js");
  const isGo = role === "backend" && project.stack.includes("Go");
  const base = project.folder.replace(/[\\/]+$/, "");
  const folder = role === "frontend" ? base : "";
  const localUrl = role === "frontend" ? project.local || "http://localhost:3000" : role === "backend" ? "http://127.0.0.1:8080" : "";
  let port: number | null = null;
  try { const parsed = new URL(localUrl); port = Number(parsed.port || (parsed.protocol === "https:" ? 443 : 80)); } catch { /* Optional URL. */ }
  const stack = serviceStackForRole(project.stack, role);
  const suggestion = suggestServiceCommands({ stack, role, runtime: "", port, commandTemplate: "auto" });
  return {
    id: crypto.randomUUID(), name: `${project.name} ${role === "frontend" ? "web" : role === "backend" ? "API" : role === "worker" ? "worker" : role === "database" ? "database" : "service"}`,
    role, stack, runtime: suggestion.runtime, folder,
    repository: "", branch: project.branch || "main", port, healthPath: role === "backend" ? "/health" : "",
    commands: suggestion.commands, commandTemplate: "auto", commandOverrides: [],
    deployment: { provider: isNext ? "Vercel" : isGo ? "Render" : "", method: isNext ? "managed" : "native", build: isNext ? "npm run build" : isGo ? "go build -o api ./cmd/api" : "", start: isGo ? "./api" : "", dockerfile: "Dockerfile", image: "" },
    environments: { local: { url: localUrl, variables: [] }, preview: { url: "", variables: [] }, production: { url: role === "frontend" ? project.url : "", variables: [] } }, updatedAt: 0,
  };
}

export function isWebUrl(value: string) {
  try { const url = new URL(value); return ["http:", "https:"].includes(url.protocol) && !url.username && !url.password; } catch { return false; }
}

export function validateService(service: ServiceProfile, portText: string, variables: Record<ServiceEnvironmentName, string>): { message: string; panel: string } | null {
  if (!service.name.trim()) return { message: "Give this service a name.", panel: "Identity" };
  if (!serviceRoles.some((item) => item.value === service.role)) return { message: "Choose a service type.", panel: "Identity" };
  if (service.repository && !isWebUrl(service.repository) && !/^git@[\w.-]+:[\w./-]+$/.test(service.repository) && !/^ssh:\/\/git@[\w.-]+(?::\d+)?\/[\w./-]+$/.test(service.repository)) return { message: "Use a repository web URL or a Git SSH address, without access tokens.", panel: "Identity" };
  if (portText.trim() && (!/^\d+$/.test(portText) || Number(portText) < 1 || Number(portText) > 65535)) return { message: "Port must be a whole number from 1 to 65535, or blank.", panel: "Local setup" };
  if (service.healthPath && !/^\/(?!\/)[^\s]*$/.test(service.healthPath)) return { message: "Health path must start with a single /, such as /health.", panel: "Local setup" };
  for (const { value, label } of environmentNames) {
    if (service.environments[value].url && !isWebUrl(service.environments[value].url)) return { message: `${label} URL must be an http:// or https:// address without credentials.`, panel: "Environments" };
    const names = variables[value].split(/[\s,]+/).filter(Boolean);
    if (names.length > 50 || names.some((name) => !/^[A-Za-z_][A-Za-z0-9_]{0,127}$/.test(name))) return { message: `${label}: enter variable names only, such as API_URL. Do not paste values or .env contents.`, panel: "Environments" };
  }
  return null;
}
