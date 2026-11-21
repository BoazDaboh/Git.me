"use client";

import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useWorkspace, updateWorkspace } from "@/lib/store";
import { downloadText, type Note } from "@/lib/workspace";
import Icon from "./Icon";
import { StorageLabel } from "./WorkspaceStorage";
import NoteBodyEditor, { type NoteBodyHandle } from "./NoteBodyEditor";

function inline(text: string): ReactNode[] {
  return text.split(/(\*\*[^*\n]+\*\*|\*[^*\n]+\*|`[^`\n]+`|\[[^\]\n]+\]\(https?:\/\/[^\s)]+\))/g).map((part, index) => {
    if (part.startsWith("**") && part.endsWith("**")) return <strong key={index}>{part.slice(2, -2)}</strong>;
    if (part.startsWith("*") && part.endsWith("*")) return <em key={index}>{part.slice(1, -1)}</em>;
    if (part.startsWith("`") && part.endsWith("`")) return <code key={index}>{part.slice(1, -1)}</code>;
    const link = part.match(/^\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)$/);
    if (link) return <a key={index} href={link[2]} target="_blank" rel="noreferrer">{link[1]}</a>;
    return part;
  });
}

function Preview({ body, onToggle }: { body: string; onToggle: (index: number, checked: boolean) => void }) {
  const lines = body.split("\n");
  const blocks: ReactNode[] = [];
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    if (line.startsWith("```")) {
      const code: string[] = [];
      while (++index < lines.length && !lines[index].startsWith("```")) code.push(lines[index]);
      blocks.push(<pre key={index}><code>{code.join("\n")}</code></pre>); continue;
    }
    const heading = line.match(/^(#{1,3})\s+(.*)/);
    if (heading) { const Tag = heading[1].length === 1 ? "h2" : heading[1].length === 2 ? "h3" : "h4"; blocks.push(<Tag key={index}>{inline(heading[2])}</Tag>); continue; }
    if (/^[-*] \[[ xX]\](?: |$)/.test(line)) { const lineIndex = index; const checked = line[3].toLowerCase() === "x"; blocks.push(<div className={`note-checklist-line ${checked ? "checked" : ""}`} key={index}><input type="checkbox" aria-label={line.slice(6) || "Checklist item"} checked={checked} onChange={(event) => onToggle(lineIndex, event.target.checked)}/><span>{inline(line.slice(6))}</span></div>); continue; }
    if (/^[-*] /.test(line)) {
      const items: ReactNode[] = [];
      do { items.push(<li key={index}>{inline(lines[index].slice(2))}</li>); index++; } while (index < lines.length && /^[-*] (?!\[)/.test(lines[index]));
      index--; blocks.push(<ul key={`list-${index}`}>{items}</ul>); continue;
    }
    if (line.startsWith("> ")) { blocks.push(<blockquote key={index}>{inline(line.slice(2))}</blockquote>); continue; }
    if (line.trim()) blocks.push(<p key={index}>{inline(line)}</p>);
    else blocks.push(<div className="note-paragraph-space" key={index}/>);
  }
  return <div className="note-preview">{body.trim() ? blocks : <span className="muted">Your note preview will appear here.</span>}</div>;
}

function Editor({ note, onUpdate, notify, onDelete }: { note: Note; onUpdate: (value: Partial<Note>) => boolean; notify: (text: string) => void; onDelete: () => void }) {
  const { projects } = useWorkspace();
  const [preview, setPreview] = useState(false);
  
  const bodyEditor = useRef<NoteBodyHandle>(null);
  function update(value: Partial<Note>) { onUpdate(value); }
  function format(action: "bold" | "italic" | "heading" | "bullet" | "checklist" | "code" | "link") {
    const field = bodyEditor.current;
    if (!field) return;
    const { start, end } = field.selection();
    const selected = note.body.slice(start, end);
    let replacement = ""; let selectStart = start; let selectEnd = end;
    if (action === "checklist") {
      const lineStart = start === 0 ? 0 : note.body.lastIndexOf("\n", start - 1) + 1;
      const rangeEnd = end > start && note.body[end - 1] === "\n" ? end - 1 : end;
      const nextBreak = note.body.indexOf("\n", rangeEnd);
      const lineEnd = nextBreak === -1 ? note.body.length : nextBreak;
      const lines = note.body.slice(lineStart, lineEnd).split("\n");
      const remove = lines.every((line) => /^[-*] \[[ xX]\](?: |$)/.test(line));
      const formatted = lines.map((line) => remove ? line.replace(/^[-*] \[[ xX]\] ?/, "") : /^[-*] \[[ xX]\](?: |$)/.test(line) ? line : `- [ ] ${line.replace(/^[-*] /, "")}`).join("\n");
      const body = note.body.slice(0, lineStart) + formatted + note.body.slice(lineEnd);
      if (body.length > 50000) return;
      update({ body });
      requestAnimationFrame(() => field.focusRange(lineStart + (remove ? 0 : 6)));
      return;
    }
    if (action === "heading" || action === "bullet") {
      const lineStart = start === 0 ? 0 : note.body.lastIndexOf("\n", start - 1) + 1;
      const prefix = action === "heading" ? "## " : "- ";
      const chunk = note.body.slice(lineStart, end);
      const formatted = chunk.split("\n").map((line) => `${prefix}${line.replace(/^[-*] \[[ xX]\] ?/, "")}`).join("\n");
      const body = note.body.slice(0, lineStart) + formatted + note.body.slice(end);
      if (body.length > 50000) return;
      update({ body });
      selectStart = start + prefix.length; selectEnd = lineStart + formatted.length;
    } else {
      const before = action === "bold" ? "**" : action === "italic" ? "*" : action === "code" ? "`" : "[";
      const after = action === "link" ? "](https://example.com)" : before;
      const content = selected || (action === "link" ? "link text" : "text");
      replacement = before + content + after;
      if (note.body.length - (end - start) + replacement.length > 50000) return;
      update({ body: note.body.slice(0, start) + replacement + note.body.slice(end) });
      selectStart = start + before.length; selectEnd = selectStart + content.length;
    }
    requestAnimationFrame(() => field.focusRange(selectStart, selectEnd));
  }
  function exportNote() {
    const name = (note.title.trim() || "untitled").replace(/[<>:"/\\|?*\x00-\x1F]/g, "-");
    downloadText(`${name}.md`, `# ${note.title || "Untitled"}\n\n${note.body}`, "text/markdown");
    notify("Note exported.");
  }
  return <section className="panel note-editor">
    <div className="note-editor-top"><div className="note-editor-meta"><span>{note.kind === "plan" ? "Plan" : "Note"}</span><span>{note.project ? projects.find(project => project.id === note.project)?.name ?? "Project unavailable" : "Other"}</span></div><div className="note-actions"><button className="icon-button delete-note" aria-label="Delete note" title="Delete note" onClick={onDelete}><Icon name="trash" size={16}/></button><button className={`icon-button ${note.pinned ? "is-pinned" : ""}`} aria-label={note.pinned ? "Unpin note" : "Pin note"} aria-pressed={note.pinned} title={note.pinned ? "Unpin note" : "Pin note"} onClick={() => update({ pinned: !note.pinned })}><Icon name="pin" size={16}/></button><button className="icon-button" aria-label="Export note" title="Export note" onClick={exportNote}><Icon name="download" size={16}/></button></div></div>
    <label className="sr-only" htmlFor="note-title">Note title</label><input id="note-title" className="note-title" value={note.title} maxLength={160} placeholder="Untitled note" onChange={(event) => update({ title: event.target.value })}/>
    <div className="note-formatting"><div className="formatting-buttons" role="toolbar" aria-label="Note formatting">{[{ action: "bold", label: "Bold", text: <b>B</b> }, { action: "italic", label: "Italic", text: <i>I</i> }, { action: "heading", label: "Heading", text: "H" }, { action: "bullet", label: "Bullet list", text: "•" }, { action: "checklist", label: "Checklist", text: <Icon name="check" size={15}/> }, { action: "code", label: "Inline code", text: "</>" }, { action: "link", label: "Link", text: <Icon name="external" size={14}/> }].map((button) => <button key={button.action} aria-label={button.label} title={button.label} disabled={preview} onMouseDown={(event) => event.preventDefault()} onClick={() => format(button.action as Parameters<typeof format>[0])}>{button.text}</button>)}</div><div className="filter-tabs note-view-tabs"><button className={!preview ? "selected" : ""} aria-pressed={!preview} onClick={() => setPreview(false)}>Write</button><button className={preview ? "selected" : ""} aria-pressed={preview} onClick={() => setPreview(true)}>Preview</button></div></div>
    {preview ? <Preview body={note.body} onToggle={(index, checked) => {
      const lines = note.body.split("\n");
      lines[index] = lines[index].replace(/^([-*] \[)[ xX](\])/, (_match, prefix, suffix) => `${prefix}${checked ? "x" : " "}${suffix}`);
      update({ body: lines.join("\n") });
    }}/> : <NoteBodyEditor ref={bodyEditor} body={note.body} onChange={(body) => update({ body })} onFormat={format}/>}
    <div className="note-editor-footer"><StorageLabel/><span>{note.body.trim() ? note.body.trim().split(/\s+/).length : 0} words</span></div>
  </section>;
}

export default function Notes({ query, notify, clearSearch, projectId, collapsibleList = false }: { query: string; notify: (text: string) => void; clearSearch: () => void; projectId?: string; collapsibleList?: boolean }) {
  const data = useWorkspace();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [listOpen, setListOpen] = useState(!collapsibleList);
  const listId = useId();
  useEffect(() => {
    if (!collapsibleList) return;
    const media = window.matchMedia("(max-width: 900px)");
    const onChange = (event: MediaQueryListEvent) => { if (event.matches) setListOpen(false); };
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, [collapsibleList]);
  const [filter, setFilter] = useState("All");
  const [creating, setCreating] = useState<Note["kind"] | null>(null);
  const [storageIssue, setStorageIssue] = useState(false);
  const needle = query.toLowerCase().trim();
  const scopeNotes = projectId ? data.notes.filter((note) => note.project === projectId) : data.notes;
  const notes = scopeNotes.filter((note) => (filter === "All" || note.kind === (filter === "Plans" ? "plan" : "note")) && `${note.title} ${note.body}`.toLowerCase().includes(needle)).sort((a, b) => Number(b.pinned) - Number(a.pinned) || b.updatedAt - a.updatedAt);
  const selected = notes.find((note) => note.id === selectedId) ?? notes[0];
  function persist(update: Parameters<typeof updateWorkspace>[0]) {
    const saved = updateWorkspace(update); setStorageIssue(!saved);
    if (!saved) notify("Browser storage unavailable. Export your notes before closing.");
    return saved;
  }
  function create(kind: Note["kind"], chosenProject: string) {
    const note: Note = { id: crypto.randomUUID(), title: kind === "plan" ? "Untitled plan" : "Untitled note", body: kind === "plan" ? "## Goals\n\n\n## Tasks\n\n- [ ] \n\n## Notes\n" : "", kind, project: chosenProject, pinned: false, updatedAt: Date.now() };
    persist((value) => ({ ...value, notes: [note, ...value.notes] })); setSelectedId(note.id); setFilter("All"); clearSearch(); setCreating(null);
  }
  return <div className={`notes-workspace ${collapsibleList ? "notes-collapsible" : ""} ${listOpen ? "" : "notes-list-collapsed"}`}>{collapsibleList && <button type="button" className="button notes-list-toggle" aria-expanded={listOpen} aria-controls={listId} onClick={() => setListOpen(open => !open)}><Icon name="menu" size={16}/>{listOpen ? "Hide notes" : "Show notes"}</button>}<div id={listId} className="notes-list panel"><div className="notes-list-heading"><h2>Notes & plans <span>{scopeNotes.length}</span></h2><div className="note-create-actions"><button className="icon-button" title="New note" aria-label="New note" onClick={() => setCreating("note")}><Icon name="plus" size={17}/></button><button className="icon-button" title="New plan" aria-label="New plan" onClick={() => setCreating("plan")}><Icon name="flow" size={16}/></button></div></div><div className="filter-tabs notes-filter">{["All", "Notes", "Plans"].map((value) => <button key={value} className={filter === value ? "selected" : ""} onClick={() => setFilter(value)}>{value}</button>)}</div><div className="note-list-items">{notes.map((note) => <button key={note.id} className={`note-list-item ${selected?.id === note.id ? "selected" : ""}`} onClick={() => { setSelectedId(note.id); if (collapsibleList && window.matchMedia("(max-width: 900px)").matches) setListOpen(false); }}><div><span className="note-kind">{note.kind === "plan" ? "PLAN" : "NOTE"}</span>{note.pinned && <Icon name="pin" size={12}/>}</div><strong>{note.title.trim() || "Untitled note"}</strong><p>{note.body.replace(/[#*`>\[\]]/g, "").trim().slice(0, 90) || "Empty note"}</p><small>{new Date(note.updatedAt).toLocaleDateString("en-GB", { day: "numeric", month: "short" })}{note.project && ` · ${data.projects.find((project) => project.id === note.project)?.name ?? "Project"}`}</small></button>)}{notes.length === 0 && <div className="note-list-empty">{scopeNotes.length ? "No matching notes." : "Your notes will appear here."}</div>}</div></div><div className="notes-editor-column">{selected ? <><Editor key={selected.id} note={selected} notify={notify} onDelete={() => { if (!window.confirm(`Delete "${selected.title || "Untitled note"}"?`)) return; persist((value) => ({ ...value, notes: value.notes.filter((note) => note.id !== selected.id) })); setSelectedId(null); }} onUpdate={(changes) => persist((value) => ({ ...value, notes: value.notes.map((note) => note.id === selected.id ? { ...note, ...changes, updatedAt: Date.now() } : note) }))}/>{storageIssue && <div className="notes-bottom"><span>Session only. Export to keep a copy.</span></div>}</> : <section className="panel notes-empty"><span className="notes-empty-icon"><Icon name="notes" size={28}/></span><h2>{scopeNotes.length ? "No matching notes" : "A place for your notes and plans"}</h2><p>{scopeNotes.length ? "Try another search or filter." : "Write freely, or start with a plan."}</p><div><button className="button primary" onClick={() => setCreating("note")}><Icon name="plus" size={15}/>New note</button><button className="button" onClick={() => setCreating("plan")}><Icon name="flow" size={15}/>New plan</button></div></section>}</div>{creating && <div className="activity-overlay" onClick={() => setCreating(null)}><section className="activity-dialog note-create-dialog" role="dialog" aria-modal="true" aria-label={`New ${creating}`} onClick={event => event.stopPropagation()}><div className="activity-heading"><h2>New {creating}</h2><button type="button" aria-label="Close" onClick={() => setCreating(null)}>×</button></div><form onSubmit={event => { event.preventDefault(); const choice = String(new FormData(event.currentTarget).get("project")); create(creating, choice === "__other__" ? "" : choice); }}><label>Project<select name="project" required defaultValue={projectId ?? ""}><option value="" disabled>Choose a project</option><option value="__other__">Other</option>{data.projects.map(project => <option value={project.id} key={project.id}>{project.name}</option>)}</select></label><div className="dialog-footer"><button type="button" className="button" onClick={() => setCreating(null)}>Cancel</button><button type="submit" className="button primary">Create {creating}</button></div></form></section></div>}</div>;
}
