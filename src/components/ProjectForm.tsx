"use client";

import { useRef, useState, type FormEvent } from "react";
import { useWorkspace, updateWorkspace } from "@/lib/store";
import { defaultProjectDirectory, stackCatalog, suggestedProjectFolder, type Project } from "@/lib/workspace";
import Icon from "./Icon";
import ProjectMark from "./ProjectMark";
import StackPicker from "./StackPicker";

export default function ProjectForm({ project, hiddenNew = false, hiddenRoot = "", onSaved, onCancel, notify }: { project?: Project; hiddenNew?: boolean; hiddenRoot?: string; onSaved: (project: Project, created: boolean) => void; onCancel: () => void; notify: (message: string) => void }) {
  const data = useWorkspace();
  const [id] = useState(() => project?.id ?? crypto.randomUUID());
  const [name, setName] = useState(project?.name ?? "");
  const [logo, setLogo] = useState(project?.logo ?? "");
  const [uploading, setUploading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const [folderOverride, setFolderOverride] = useState<string | null>(project?.folder ?? null);
  const [projectMode, setProjectMode] = useState<"existing" | "template">("existing");
  const [frontend, setFrontend] = useState("next");
  const [backend, setBackend] = useState("go");
  const [stack, setStack] = useState(project?.stack ?? []);
  const [stackQuery, setStackQuery] = useState("");
  const uploadRef = useRef<HTMLInputElement>(null);
  const base = hiddenNew ? hiddenRoot : data.projectDirectory || defaultProjectDirectory;
  const folder = folderOverride ?? suggestedProjectFolder(base, name);
  const catalog = [...new Set([...stackCatalog, ...data.projects.flatMap((item) => item.stack)])];

  async function readLogo(file?: File) {
    if (!file) return;
    if (!["image/png", "image/jpeg", "image/webp"].includes(file.type) || file.size > 2 * 1024 * 1024) { setError("Choose a PNG, JPG or WebP image up to 2 MB."); return; }
    setUploading(true); setError("");
    let bitmap: ImageBitmap | undefined;
    try {
      bitmap = await createImageBitmap(file);
      const canvas = document.createElement("canvas"); canvas.width = 192; canvas.height = 192;
      const context = canvas.getContext("2d"); if (!context) throw new Error("Canvas unavailable");
      const scale = Math.min(192 / bitmap.width, 192 / bitmap.height);
      const width = bitmap.width * scale, height = bitmap.height * scale;
      context.drawImage(bitmap, (192 - width) / 2, (192 - height) / 2, width, height);
      setLogo(canvas.toDataURL("image/webp", 0.9));
    } catch { setError("That image could not be opened. Try another file."); }
    finally { bitmap?.close(); setUploading(false); }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError("");
    const form = new FormData(event.currentTarget);
    const cleanName = name.trim(); if (!cleanName || uploading || creating) return;
    const url = String(form.get("url") ?? "").trim(), local = String(form.get("local") ?? "").trim();
    if ([url, local].some((value) => value && !/^https?:\/\//i.test(value))) { setError("Use an http:// or https:// address."); return; }
    const pending = stackQuery.trim();
    const pendingName = catalog.find((item) => item.toLowerCase() === pending.toLowerCase()) ?? pending;
    const finalStack = [...stack]; if (pendingName && !finalStack.some((item) => item.toLowerCase() === pendingName.toLowerCase())) finalStack.push(pendingName);
    const value: Project = { ...project, ...(hiddenNew ? { hidden: true } : {}), id, name: cleanName, initials: project?.initials ?? cleanName.slice(0, 2).toLowerCase(), color: project?.color ?? "blue", description: String(form.get("description") ?? "").trim(), stack: finalStack.slice(0, 20), stage: project?.stage ?? "Planning", progress: project?.progress ?? 0, branch: String(form.get("branch") ?? "main").trim() || "main", folder: folder.trim(), url, local, note: project?.note ?? "", logo: logo || undefined };
    if (!project && projectMode === "template") {
      if (!folder.trim()) { setError("Choose a project folder."); return; }
      if (!window.confirm(`Create the displayed template in:\n\n${folder.trim()}\n\nExisting folders will not be overwritten. No packages will be installed.`)) return;
      setCreating(true);
      try {
        const response = await fetch("http://127.0.0.1:47821/projects/create-template", { method:"POST", headers:{"Content-Type":"application/json","X-GitMe-Client":"workspace-v1"}, body:JSON.stringify({folder:folder.trim(),frontend,backend}) });
        if (!response.ok) throw new Error((await response.text()).trim() || "Could not create folder.");
      } catch (error) { setError(error instanceof TypeError ? "Could not reach the Go companion. Check the folder before retrying, then restart the updated companion." : error instanceof Error ? error.message : "Could not create the folder. Inspect the destination before retrying."); setCreating(false); return; }
      setCreating(false);
    }
    const persisted = updateWorkspace((current) => ({ ...current, projects: current.projects.some((item) => item.id === id) ? current.projects.map((item) => item.id === id ? value : item) : [...current.projects, value] }));
    if (!persisted) { setError(!project && projectMode === "template" ? "Folder created, but the project record is only in this session. Export your workspace and check storage before retrying." : "Changes are only in this session. Remove a large logo or export your workspace to keep a copy."); return; }
    notify(project ? "Project updated." : "Project created."); onSaved(value, !project);
  }

  return <form className="editor-form project-form" onSubmit={submit}>
    <div className="logo-field"><ProjectMark project={{ initials: (name || "gm").slice(0, 2).toLowerCase(), color: "blue", logo }} large/><div><div className="logo-field-actions"><button className="button" type="button" onClick={() => uploadRef.current?.click()} disabled={uploading}><Icon name="plus" size={14}/>{uploading ? "Opening image..." : "Upload logo"}</button>{logo && <button type="button" className="text-link" onClick={() => setLogo("")}>Use initials</button>}</div><p>PNG, JPG or WebP · Up to 2 MB</p><input ref={uploadRef} type="file" className="sr-only" aria-label="Project logo" accept="image/png,image/jpeg,image/webp" onChange={(event) => { void readLogo(event.target.files?.[0]); event.target.value = ""; }}/></div></div>
    <label>Project name<input name="name" required maxLength={60} value={name} onChange={(event) => setName(event.target.value)} autoFocus placeholder="Your next project"/></label>
    <label>Description<input name="description" maxLength={180} defaultValue={project?.description} placeholder="What are you building?"/></label>
    {!project && <section className="project-template-picker" aria-label="Project setup">
      <label>Project setup<select value={projectMode} onChange={(event) => setProjectMode(event.target.value as "existing" | "template")}><option value="existing">Use my own folder</option><option value="template">Create from a folder template</option></select></label>
      {projectMode === "template" && <><div className="form-row"><label>Frontend<select value={frontend} onChange={(event) => setFrontend(event.target.value)}><option value="next">Next.js + TypeScript</option><option value="react">React + TypeScript</option><option value="static">HTML, CSS + JavaScript</option></select></label><label>Backend API<select value={backend} onChange={(event) => setBackend(event.target.value)}><option value="go">Go</option><option value="node">Node.js</option><option value="php">PHP</option></select></label></div>
        <div className="project-template-preview"><strong>Folder preview</strong><code>{folder || "Project folder"}</code><pre>{["├── README.md", "├── .gitignore", "├── private/", "├── web/", ...(frontend === "static" ? ["│   ├── index.html", "│   ├── css/", "│   └── js/"] : ["│   ├── public/", "│   └── src/", ...(frontend === "next" ? ["│       ├── app/", "│       └── components/"] : ["│       └── components/"])]), "└── api/", ...(backend === "go" ? ["    ├── cmd/server/", "    ├── internal/", "    └── migrations/"] : backend === "node" ? ["    ├── src/", "    └── tests/"] : ["    ├── public/", "    ├── src/", "    └── tests/"])].join("\n")}</pre><p>Creates the folder structure and starter files. Frameworks and dependencies are not installed.</p></div></>}
    </section>}
    <StackPicker selected={stack} onChange={setStack} query={stackQuery} onQueryChange={setStackQuery} catalog={catalog}/>
    <div className="directory-field"><label>Local folder<input name="folder" maxLength={500} value={folder} onChange={(event) => setFolderOverride(event.target.value)}/></label><div><span>{projectMode === "template" && !project ? "The parent folder must exist. The destination must be new." : "Saving a path does not create the folder."}</span><button type="button" className="text-link" onClick={() => setFolderOverride(null)}>Use suggested path</button></div></div>
    <div className="form-row"><label>Local URL<input name="local" type="url" defaultValue={project?.local} placeholder="http://localhost:3000"/></label><label>Live URL<input name="url" type="url" defaultValue={project?.url} placeholder="https://..."/></label></div>
    <label>Git branch<input name="branch" maxLength={100} defaultValue={project?.branch ?? "main"}/></label>
    {error && <p className="project-form-error" role="alert">{error}</p>}
    <div className="dialog-footer"><button className="button" type="button" onClick={onCancel} disabled={creating}>Cancel</button><button className="button primary" type="submit" disabled={uploading || creating}>{creating ? "Creating..." : project ? "Save changes" : "Create project"}<Icon name="arrow" size={15}/></button></div>
  </form>;
}
