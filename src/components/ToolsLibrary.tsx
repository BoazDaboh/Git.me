"use client";
import { useRef, useState, type FormEvent } from "react";
import { downloadText, tools, type LibraryItem } from "@/lib/workspace";
import { updateWorkspace, useWorkspace } from "@/lib/store";
import Icon from "./Icon";

const folders = ["Links", "Skills", "Notes", "Commands"] as const;
type Folder = typeof folders[number];
const categories = ["General", "Design", "Inspiration", "Frontend", "Backend", "Workflow", "Hosting"];

export default function ToolsLibrary({ query, notify }: { query: string; notify: (message: string) => void }) {
  const data = useWorkspace();
  const [folder, setFolder] = useState<Folder | null>(null);
  const [category, setCategory] = useState("All");
  const [editing, setEditing] = useState<LibraryItem | "new" | null>(null);
  const [viewing, setViewing] = useState<LibraryItem | null>(null);
  const skillFile = useRef<HTMLInputElement>(null);
  const personal = data.libraryItems ?? [];
  const defaults: LibraryItem[] = tools.map(item => ({ id: `default:${item.name}`, folder: "Links", name: item.name, category: item.category, content: item.url, description: item.description }));
  const all = [...defaults, ...personal];
  const list = all.filter(item => item.folder === folder && (category === "All" || item.category === category) && `${item.name} ${item.description} ${item.content} ${item.category}`.toLowerCase().includes(query.toLowerCase().trim()));
  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!folder || !editing) return;
    const form = new FormData(event.currentTarget);
    const name = String(form.get("name") ?? "").trim(), content = String(form.get("content") ?? "").trim();
    if (!name || !content) return;
    if (folder === "Links") { try { const url = new URL(content); if (!["https:", "http:"].includes(url.protocol)) throw Error(); } catch { notify("Enter a valid http or https link."); return; } }
    const item: LibraryItem = { id: editing === "new" ? crypto.randomUUID() : editing.id, folder, name, content, category: String(form.get("category") ?? "General"), description: String(form.get("description") ?? "").trim() };
    const saved = updateWorkspace(value => ({ ...value, libraryItems: [...(value.libraryItems ?? []).filter(entry => entry.id !== item.id), item] }));
    if (!saved) notify("Browser storage unavailable. Changes are pending.");
    setEditing(null);
  }
  async function importSkill(file: File) {
    if (!file.name.toLowerCase().endsWith(".md") || file.size > 100000) { notify("Choose a Markdown file under 100 KB."); return; }
    try {
      const content = await file.text();
      if (!content.trim() || content.length > 50000) { notify("The skill file is empty or too long."); return; }
      const item: LibraryItem = { id: crypto.randomUUID(), folder: "Skills", name: file.name.replace(/\.md$/i, ""), category: "General", description: "", content };
      if (!updateWorkspace(value => ({ ...value, libraryItems: [...(value.libraryItems ?? []), item] }))) notify("Browser storage unavailable. Changes are pending.");
      else notify("Skill imported. Wait for Saved to SQLite.");
    } catch { notify("Could not read the skill file."); }
  }
  async function copy(value: string) { try { await navigator.clipboard.writeText(value); notify("Copied."); } catch { notify("Clipboard unavailable."); } }
  return <div className="library">{!folder ? <div className="library-folders">{folders.map(name => <button className="panel library-folder" key={name} onClick={() => { setFolder(name); setCategory("All"); }}><span className="quick-icon"><Icon name={name === "Links" ? "link" : name === "Commands" ? "terminal" : name === "Skills" ? "file" : "notes"}/></span><strong>{name}</strong><span>{all.filter(item => item.folder === name).length}</span><Icon name="chevron" size={16}/></button>)}</div> : <><div className="library-toolbar"><button className="text-link" onClick={() => setFolder(null)}>← Folders</button><h2>{folder}</h2><select aria-label="Filter by category" value={category} onChange={event => setCategory(event.target.value)}><option>All</option>{[...new Set([...categories, ...all.map(item => item.category)])].map(item => <option key={item}>{item}</option>)}</select>{folder === "Skills" && <><input ref={skillFile} type="file" accept=".md,text/markdown,text/plain" className="sr-only" aria-label="Import skill Markdown file" onChange={event => { const file = event.target.files?.[0]; event.target.value = ""; if (file) void importSkill(file); }}/><button className="button" onClick={() => skillFile.current?.click()}><Icon name="upload" size={15}/>Import skill</button></>}<button className="button primary" onClick={() => setEditing("new")}><Icon name="plus" size={15}/>Add {folder === "Links" ? "link" : folder === "Skills" ? "skill" : folder === "Notes" ? "note" : "command"}</button></div><div className="library-items">{list.map(item => <article className="panel library-item" key={item.id}><div><strong>{item.name}</strong><span className="badge">{item.category}</span></div>{item.description && <p>{item.description}</p>}{item.folder === "Links" ? <a className="text-link" href={item.content} target="_blank" rel="noopener noreferrer">Open <Icon name="external" size={14}/></a> : <button className="text-link" onClick={() => setViewing(item)}>View <Icon name="arrow" size={14}/></button>}<div className="library-item-actions">{item.folder === "Skills" && <button className="icon-button" title="Export SKILL.md" aria-label={`Export ${item.name} as SKILL.md`} onClick={() => downloadText("SKILL.md", item.content, "text/markdown")}><Icon name="download" size={15}/></button>}{item.folder !== "Links" && <button className="icon-button" title="Copy" aria-label={`Copy ${item.name}`} onClick={() => void copy(item.content)}><Icon name="copy" size={15}/></button>}{!item.id.startsWith("default:") && <><button className="text-link" onClick={() => setEditing(item)}>Edit</button><button className="icon-button" aria-label={`Delete ${item.name}`} title="Delete" onClick={() => { if (window.confirm(`Delete ${item.name}?`)) updateWorkspace(value => ({ ...value, libraryItems: (value.libraryItems ?? []).filter(entry => entry.id !== item.id) })); }}><Icon name="trash" size={15}/></button></>}</div></article>)}{!list.length && <div className="panel library-empty">No items found.</div>}</div></>}
  {editing && folder && <div className="activity-overlay" onClick={() => setEditing(null)}><section className="activity-dialog library-dialog" role="dialog" aria-modal="true" aria-label={`${editing === "new" ? "Add" : "Edit"} ${folder}`} onClick={event => event.stopPropagation()}><div className="activity-heading"><h2>{editing === "new" ? "Add" : "Edit"} {folder.toLowerCase()}</h2><button aria-label="Close" onClick={() => setEditing(null)}>×</button></div><form onSubmit={save}><label>Name<input name="name" required maxLength={160} defaultValue={editing === "new" ? "" : editing.name}/></label><label>Category<select name="category" defaultValue={editing === "new" ? "General" : editing.category}>{categories.map(item => <option key={item}>{item}</option>)}</select></label><label>Description<input name="description" maxLength={300} defaultValue={editing === "new" ? "" : editing.description}/></label><label>{folder === "Links" ? "URL" : folder === "Skills" ? "SKILL.md content" : folder === "Commands" ? "Command" : "Content"}{folder === "Links" ? <input name="content" type="url" required defaultValue={editing === "new" ? "" : editing.content}/> : <textarea name="content" required rows={8} maxLength={50000} defaultValue={editing === "new" ? "" : editing.content}/>}</label><div className="dialog-footer"><button type="button" className="button" onClick={() => setEditing(null)}>Cancel</button><button className="button primary" type="submit">Save</button></div></form></section></div>}
  {viewing && <div className="activity-overlay" onClick={() => setViewing(null)}><section className="activity-dialog library-dialog" role="dialog" aria-modal="true" aria-label={viewing.name} onClick={event => event.stopPropagation()}><div className="activity-heading"><h2>{viewing.name}</h2><button aria-label="Close" onClick={() => setViewing(null)}>×</button></div><pre>{viewing.content}</pre><button className="button" onClick={() => void copy(viewing.content)}><Icon name="copy" size={15}/>Copy</button></section></div>}</div>;
}
