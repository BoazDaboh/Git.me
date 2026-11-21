"use client";

import { useRef, useState, type FormEvent } from "react";
import { useWorkspace, updateWorkspace } from "@/lib/store";
import { defaultProjectDirectory, projectStages, stackCatalog, suggestedProjectFolder, type Project } from "@/lib/workspace";
import Icon from "./Icon";
import ProjectMark from "./ProjectMark";
import StackPicker from "./StackPicker";

export default function ProjectForm({ project, onSaved, onCancel, notify }: { project?: Project; onSaved: (project: Project, created: boolean) => void; onCancel: () => void; notify: (message: string) => void }) {
  const data = useWorkspace();
  const [id] = useState(() => project?.id ?? crypto.randomUUID());
  const [name, setName] = useState(project?.name ?? "");
  const [logo, setLogo] = useState(project?.logo ?? "");
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const [folderOverride, setFolderOverride] = useState<string | null>(project?.folder ?? null);
  const [stack, setStack] = useState(project?.stack ?? []);
  const [stackQuery, setStackQuery] = useState("");
  const uploadRef = useRef<HTMLInputElement>(null);
  const base = data.projectDirectory || defaultProjectDirectory;
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

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError("");
    const form = new FormData(event.currentTarget);
    const cleanName = name.trim(); if (!cleanName || uploading) return;
    const url = String(form.get("url") ?? "").trim(), local = String(form.get("local") ?? "").trim();
    if ([url, local].some((value) => value && !/^https?:\/\//i.test(value))) { setError("Use an http:// or https:// address."); return; }
    const pending = stackQuery.trim();
    const pendingName = catalog.find((item) => item.toLowerCase() === pending.toLowerCase()) ?? pending;
    const finalStack = [...stack]; if (pendingName && !finalStack.some((item) => item.toLowerCase() === pendingName.toLowerCase())) finalStack.push(pendingName);
    const progress = Number(form.get("progress"));
    if (!Number.isFinite(progress) || progress < 0 || progress > 100) { setError("Progress must be between 0 and 100."); return; }
    const value: Project = { ...project, id, name: cleanName, initials: project?.initials ?? cleanName.slice(0, 2).toLowerCase(), color: project?.color ?? "blue", description: String(form.get("description") ?? "").trim(), stack: finalStack.slice(0, 20), stage: String(form.get("stage")), progress, branch: String(form.get("branch") ?? "main").trim() || "main", folder: folder.trim(), url, local, note: project?.note ?? "", logo: logo || undefined };
    const persisted = updateWorkspace((current) => ({ ...current, projects: current.projects.some((item) => item.id === id) ? current.projects.map((item) => item.id === id ? value : item) : [...current.projects, value] }));
    if (!persisted) { setError("Changes are only in this session. Remove a large logo or export your workspace to keep a copy."); return; }
    notify(project ? "Project updated." : "Project created."); onSaved(value, !project);
  }

  return <form className="editor-form project-form" onSubmit={submit}>
    <div className="logo-field"><ProjectMark project={{ initials: (name || "gm").slice(0, 2).toLowerCase(), color: "blue", logo }} large/><div><div className="logo-field-actions"><button className="button" type="button" onClick={() => uploadRef.current?.click()} disabled={uploading}><Icon name="plus" size={14}/>{uploading ? "Opening image..." : "Upload logo"}</button>{logo && <button type="button" className="text-link" onClick={() => setLogo("")}>Use initials</button>}</div><p>PNG, JPG or WebP · Up to 2 MB</p><input ref={uploadRef} type="file" className="sr-only" aria-label="Project logo" accept="image/png,image/jpeg,image/webp" onChange={(event) => { void readLogo(event.target.files?.[0]); event.target.value = ""; }}/></div></div>
    <label>Project name<input name="name" required maxLength={60} value={name} onChange={(event) => setName(event.target.value)} autoFocus placeholder="Your next project"/></label>
    <label>Description<input name="description" maxLength={180} defaultValue={project?.description} placeholder="What are you building?"/></label>
    <div className="form-row"><label>Stage<select name="stage" defaultValue={project?.stage ?? "Planning"}>{projectStages.map((stage) => <option key={stage}>{stage}</option>)}</select></label><label>Progress (%)<input name="progress" type="number" min={0} max={100} step={1} defaultValue={project?.progress ?? 0}/></label></div>
    <StackPicker selected={stack} onChange={setStack} query={stackQuery} onQueryChange={setStackQuery} catalog={catalog}/>
    <div className="directory-field"><label>Local folder<input name="folder" maxLength={500} value={folder} onChange={(event) => setFolderOverride(event.target.value)}/></label><div><span>Saving a path does not create the folder.</span><button type="button" className="text-link" onClick={() => setFolderOverride(null)}>Use suggested path</button></div></div>
    <div className="form-row"><label>Local URL<input name="local" type="url" defaultValue={project?.local} placeholder="http://localhost:3000"/></label><label>Live URL<input name="url" type="url" defaultValue={project?.url} placeholder="https://..."/></label></div>
    <label>Git branch<input name="branch" maxLength={100} defaultValue={project?.branch ?? "main"}/></label>
    {error && <p className="project-form-error" role="alert">{error}</p>}
    <div className="dialog-footer"><button className="button" type="button" onClick={onCancel}>Cancel</button><button className="button primary" type="submit" disabled={uploading}>{project ? "Save changes" : "Create project"}<Icon name="arrow" size={15}/></button></div>
  </form>;
}
