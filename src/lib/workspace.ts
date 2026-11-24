import type { ServiceProfile } from "./services";
import type { GitHubLink } from "./github";
import type { VercelLink } from "./vercel";
import type { SavedApiRequest } from "./api-tester";

export const sections = ["projects", "flow", "debug", "notes", "lessons", "tools", "publishing", "reports", "pdf", "settings"] as const;
export type Section = "overview" | (typeof sections)[number];
export type Project = {
  id: string; name: string; initials: string; color: string; description: string;
  stack: string[]; stage: string; progress: number; branch: string;
  folder: string; url: string; local: string; note: string; logo?: string; pinned?: boolean; hidden?: boolean; services?: ServiceProfile[]; unassignedGitHubLinks?: GitHubLink[]; vercel?: VercelLink; apiRequests?:SavedApiRequest[];
};
export type Task = { id: string; title: string; project: string; status: "Planned" | "In progress" | "Done"; priority: string; scheduledDate?: string };
export type LibraryItem = { id: string; folder: "Links" | "Skills" | "Notes" | "Commands"; name: string; category: string; content: string; description: string };
export type Tool = { name: string; category: string; description: string; url: string; initials: string; color: string };
export type Draft = { id: string; platform: string; project: string; text: string };
export type Session = { project: string; started: number; ended: number };
export type Note = { id: string; title: string; body: string; kind: "note" | "plan"; project: string; pinned: boolean; updatedAt: number; scheduledDate?: string; planStatus?: "planned" | "in-progress" | "done" | "cancelled" };
export type PdfTextStyle = { alignment: "left" | "center" | "right"; fontSize: number; bold: boolean; italic: boolean };
export type PostContext = { projectId: string; projectName: string; description: string; repository: string; commits: { sha: string; title: string; date: string }[]; extra: string; platform: string; tone: string; savedAt: number };
export type CvDraft = { template: "classic" | "compact"; name: string; role: string; email: string; phone: string; location: string; contact?: string; website: string; profile: string; experience: string; education: string; skills: string; projects?: string; leadership?: string; achievements?: string; sectionTitles?: Partial<Record<"profile" | "skills" | "education" | "experience" | "projects" | "leadership" | "achievements", string>>; richText?: Record<string, unknown>; customSections?: { id: string; title: string; content: string }[] };
export type SavedCv = { id: string; title: string; updatedAt: number; draft: CvDraft };
export type Lesson = { id: string; title: string; category: string; body: string; updatedAt: number };
export type WorkspaceData = { version: 1; workspaceName?: string; projects: Project[]; tasks: Task[]; drafts: Draft[]; notes: Note[]; sessions: Session[]; projectDirectory?: string; collapsedTaskGroups?: string[]; libraryItems?: LibraryItem[]; lessons?: Lesson[]; postContexts?: PostContext[]; cvDraft?: CvDraft; cvSaves?: SavedCv[]; cvActiveId?: string; focus: { project: string; started: number; elapsedMs?: number } | null; document: { title: string; text: string; subtitle?: string; footer?: string; logo?: string; logoAlignment?: "left" | "center" | "right"; alignment?: "left" | "center" | "right"; fontSize?: number; bold?: boolean; italic?: boolean; blocks?: { id: string; text: string; style: PdfTextStyle }[]; styles?: Partial<Record<"title" | "subtitle" | "text" | "footer", PdfTextStyle>> } };

export const defaultProjectDirectory = "";
export const projectStages = ["Planning", "Building", "Review", "Live", "Maintenance"];
export const stackCatalog = ["TypeScript", "JavaScript", "React", "Next.js", "Node.js", "NestJS", "Express", "Vite", "Go", "PHP", "Composer", "Laravel", "PostgreSQL", "MySQL", "SQLite", "Redis", "HTML", "CSS", "Tailwind CSS", "Vue", "Nuxt", "Svelte", "Astro", "Python", "Django", "FastAPI", "Docker", "React Native", "Expo", "Flutter", "Swift", "Kotlin"];
export const projectHref = (id: string) => `/projects/${encodeURIComponent(id)}`;
export function suggestedProjectFolder(base: string, name: string) {
  if (!base.trim()) return "";
  const slug = name.normalize("NFKD").replace(/[\u0300-\u036f]/g, "").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "").slice(0, 70) || "new-project";
  return `${base.replace(/[\\/]+$/, "")}\\${slug}`;
}

export const navigation: { id: Section; label: string; icon: string; group: string }[] = [
  { id: "overview", label: "Overview", icon: "grid", group: "WORKSPACE" },
  { id: "projects", label: "Projects", icon: "folder", group: "WORKSPACE" },
  { id: "flow", label: "Tasks & flow", icon: "flow", group: "WORKSPACE" },
  { id: "debug", label: "Debug & tests", icon: "terminal", group: "WORKSPACE" },
  { id: "notes", label: "Notes & plans", icon: "notes", group: "WORKSPACE" },
  { id: "lessons", label: "Lessons", icon: "book", group: "RESOURCES" },
  { id: "tools", label: "Tools library", icon: "tools", group: "RESOURCES" },
  { id: "publishing", label: "Portfolio & posts", icon: "send", group: "RESOURCES" },
  { id: "reports", label: "Reports", icon: "chart", group: "RESOURCES" },
  { id: "pdf", label: "PDF workspace", icon: "file", group: "RESOURCES" },
];

export const seed: WorkspaceData = {
  version: 1,
  projects: [],
  tasks: [],
  drafts: [],
  sessions: [], notes: [], focus: null,
  document: { title: "Project brief", text: "Project overview\n\nDescribe what you are building and who it is for.\n\nScope\n\nList the features included in this milestone.\n\nNext steps\n\nAdd your next action." },
};

export const tools: Tool[] = [
  { name: "Figma", category: "Design", description: "Design files and interface exploration", url: "https://www.figma.com/", initials: "Fi", color: "purple" },
  { name: "Mobbin", category: "Inspiration", description: "Real product interfaces and flows", url: "https://mobbin.com/", initials: "M", color: "dark" },
  { name: "Notboring", category: "Inspiration", description: "Interface references and design resources", url: "https://notboring.dev/", initials: "nb", color: "orange" },
  { name: "Next.js", category: "Frontend", description: "App Router and framework documentation", url: "https://nextjs.org/docs", initials: "N", color: "dark" },
  { name: "React", category: "Frontend", description: "Components, state and UI fundamentals", url: "https://react.dev/", initials: "R", color: "blue" },
  { name: "Go", category: "Backend", description: "Language reference and standard library", url: "https://go.dev/doc/", initials: "Go", color: "blue" },
  { name: "GitHub", category: "Workflow", description: "Repositories, issues and pull requests", url: "https://github.com/", initials: "gh", color: "dark" },
  { name: "Vercel", category: "Hosting", description: "Frontend deployments and previews", url: "https://vercel.com/dashboard", initials: "V", color: "dark" },
  { name: "Render", category: "Hosting", description: "Backend services and deployment logs", url: "https://dashboard.render.com/", initials: "Re", color: "green" },
];

export function downloadText(name: string, text: string, type = "text/plain") {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const anchor = document.createElement("a"); anchor.href = url; anchor.download = name; anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function clock(seconds: number) {
  const value = Math.max(0, Math.floor(seconds));
  return `${String(Math.floor(value / 3600)).padStart(2, "0")}:${String(Math.floor(value / 60) % 60).padStart(2, "0")}:${String(value % 60).padStart(2, "0")}`;
}
