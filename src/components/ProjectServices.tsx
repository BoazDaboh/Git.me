"use client";

import { useRef, useState, type FormEvent } from "react";
import { useWorkspace, updateWorkspace } from "@/lib/store";
import { createServiceDraft, isWebUrl, serviceCommands, serviceRoles, serviceScope, type ServiceProfile, type ServiceScope } from "@/lib/services";
import Icon from "./Icon";
import ServiceEditor from "./ServiceEditor";
import ServiceRuntime from "./ServiceRuntime";
import GitHubRepository from "./GitHubRepository";





const scopeLabels = { frontend: "Frontend", backend: "Backend", other: "Other services" };
const scopeIcons = { frontend: "monitor", backend: "terminal", other: "tools" };

function Value({ value }: { value: string | number | null }) { return value !== "" && value !== null ? <span>{value}</span> : <span className="muted">Not set</span>; }

function ServiceTitle({ projectId, profile, disabled }: { projectId: string; profile: ServiceProfile; disabled: boolean }) {
  const [editing,setEditing] = useState(false);
  const [title,setTitle] = useState(profile.name);
  const [error,setError] = useState("");
  function save(event: FormEvent) {
    event.preventDefault(); if (disabled) return;
    const name = title.trim();
    if (!name || name.length > 80) { setError("Enter a title of 1 to 80 characters."); return; }
    const saved = updateWorkspace((data) => ({ ...data, projects: data.projects.map((project) => project.id !== projectId ? project : { ...project, services: (project.services || []).map((service) => service.id === profile.id ? { ...service, name, updatedAt: Date.now() } : service) }) }));
    if (saved) setEditing(false); else setError("Title changed for this session only. Browser storage is unavailable.");
  }
  if (!editing) return <div className="service-title"><h2>{profile.name}</h2><button className="text-link" disabled={disabled} title={disabled ? "Stop the managed service before renaming." : undefined} aria-label="Rename service title" onClick={() => { setTitle(profile.name);setError("");setEditing(true); }}><Icon name="edit" size={13}/>Rename</button></div>;
  return <form className="service-title-editor" onSubmit={save}><label>Service title<input value={title} maxLength={80} placeholder="Test, Production, Review..." disabled={disabled} onChange={(event) => { setTitle(event.target.value);setError(""); }} autoFocus/></label><div><button className="button integration-secondary" type="submit" disabled={disabled}><Icon name="check" size={13}/>Save title</button><button className="button" type="button" onClick={() => setEditing(false)}>Cancel</button></div>{error && <p className="github-error" role="alert">{error}</p>}</form>;
}

function ServiceRecord({ projectId, profile, edit, remove, copy, openDeploy }: { projectId: string; profile: ServiceProfile; edit: () => void; remove: () => void; copy: (value: string) => void; openDeploy: () => void }) {
  const [managedActive, setManagedActive] = useState(false);

  const config = profile.environments.local;
  return <div className="service-record">
    <section className="panel service-record-summary"><div className="panel-heading"><div><span className="service-record-type">{serviceRoles.find((role) => role.value === profile.role)?.label}</span><ServiceTitle projectId={projectId} profile={profile} disabled={managedActive}/></div><div className="service-record-actions"><button className="button" disabled={managedActive} title={managedActive ? "Stop the managed service before editing." : undefined} onClick={edit}>Edit profile<Icon name="arrow" size={14}/></button><button className="icon-button" aria-label={`Delete ${profile.name} profile`} title={managedActive ? "Stop the managed service before deleting." : "Delete profile"} disabled={managedActive} onClick={remove}><Icon name="trash" size={15}/></button></div></div><div className="service-record-status"><span className="badge"><i/>Service profile</span><span>Local configuration</span><span className="service-saved-at">Saved {new Date(profile.updatedAt).toLocaleDateString("en-GB", { day: "numeric", month: "short" })}</span></div><div className="service-config-metrics">{[{ label: "Runtime", value: profile.runtime }, { label: "Local port", value: profile.port }, { label: "Branch", value: profile.branch }, { label: "GitHub", value: profile.github?.fullName || "Not linked" }].map((item) => <div key={item.label}><small>{item.label}</small><strong><Value value={item.value}/></strong></div>)}</div>{profile.stack.length > 0 && <div className="service-stack tags">{profile.stack.map((item) => <span key={item}>{item}</span>)}</div>}</section>
    <GitHubRepository projectId={projectId} serviceId={profile.id}/>
    <section className="panel service-deploy-shortcut"><div><h2>Deployment</h2><p>{profile.vercel ? `Vercel · ${profile.vercel.scopeSlug}/${profile.vercel.projectName}` : "Manage hosting, releases and activity in Deploy."}</p></div><button className="button integration-action" onClick={openDeploy}>Open deployment<Icon name="arrow" size={14}/></button></section>
    <ServiceRuntime projectId={projectId} profile={profile} onActiveChange={setManagedActive}/>
    <section className="panel service-locations"><div className="panel-heading"><h2>Source & local setup</h2><Icon name="folder" size={16}/></div><dl>{[{ label: "Service folder", value: profile.folder, mono: true }, { label: "Repository", value: profile.repository, mono: true }, { label: "Branch", value: profile.branch, mono: true }, { label: "Health path", value: profile.healthPath, mono: true }].map((item) => <div key={item.label}><dt>{item.label}</dt><dd><code><Value value={item.value}/></code>{item.value && <button className="icon-button" aria-label={`Copy ${item.label.toLowerCase()}`} onClick={() => copy(item.value)}><Icon name="copy" size={14}/></button>}</dd></div>)}</dl></section>
    <section className="panel service-environment-panel"><div className="panel-heading"><h2>Local environment</h2></div><div className="service-environment-body"><div className="service-environment-url"><small>Local URL</small><div>{config.url && isWebUrl(config.url) ? <a href={config.url} target="_blank" rel="noreferrer">{config.url}<Icon name="external" size={13}/></a> : <span className="muted">Not set</span>}{config.url && <button className="icon-button" aria-label="Copy local URL" onClick={() => copy(config.url)}><Icon name="copy" size={14}/></button>}</div></div><div className="service-variable-names"><small>Required variable names</small>{config.variables.length ? <div className="tags">{config.variables.map((name) => <span key={name}>{name}</span>)}</div> : <span className="muted">None recorded</span>}</div></div></section>
    <section className="panel service-command-panel"><div className="panel-heading"><h2>Local commands</h2><span className="muted">Configured</span></div>{serviceCommands.map((command) => <div className="service-command-row" key={command.value}><span>{command.label}</span><code><Value value={profile.commands[command.value]}/></code>{profile.commands[command.value] && <button className="icon-button" aria-label={`Copy ${command.label.toLowerCase()} command`} onClick={() => copy(profile.commands[command.value])}><Icon name="copy" size={14}/></button>}</div>)}</section>
  </div>;
}

export default function ProjectServices({ projectId, scope, query, selectedId, onSelect, onSaved, notify, copy, openDeploy }: { projectId: string; scope: ServiceScope; query: string; selectedId: string | null; onSelect: (id: string) => void; onSaved: (profile: ServiceProfile) => void; notify: (message: string) => void; copy: (value: string) => void; openDeploy: (profile: ServiceProfile) => void }) {
  const data = useWorkspace();
  const workspaceRef = useRef<HTMLDivElement>(null);
  const [editor, setEditor] = useState<{ draft: ServiceProfile; existing: boolean } | null>(null);
  const project = data.projects.find((item) => item.id === projectId);
  if (!project) return null;
  const allProfiles = project.services ?? [];
  const profiles = allProfiles.filter((profile) => serviceScope(profile.role) === scope && `${profile.name} ${profile.runtime} ${profile.stack.join(" ")}`.toLowerCase().includes(query.toLowerCase().trim()));
  const active = profiles.find((profile) => profile.id === selectedId) ?? profiles[0];
  const start = () => setEditor({ draft: createServiceDraft(project, scope === "other" ? "worker" : scope), existing: false });
  return <div ref={workspaceRef} className="project-services"><div className="section-controls"><span className="section-label"><Icon name={scopeIcons[scope]} size={16}/>{scopeLabels[scope]} profiles <span className="muted">{profiles.length}</span></span><button className="button primary" onClick={start}><Icon name="plus" size={15}/>Add service</button></div>
    {profiles.length ? <div className="service-workspace"><aside className="panel service-profile-list" aria-label={`${scopeLabels[scope]} services`}><div className="service-list-heading">SERVICES</div>{profiles.map((profile) => <button key={profile.id} className={`service-profile-item ${active?.id === profile.id ? "selected" : ""}`} aria-pressed={active?.id === profile.id} onClick={() => onSelect(profile.id)}><span className="quick-icon"><Icon name={profile.role === "frontend" ? "monitor" : "terminal"} size={16}/></span><span><strong>{profile.name}</strong><small>{profile.runtime || "Runtime not set"}</small></span><Icon name="chevron" size={13}/></button>)}</aside>{active && <ServiceRecord key={active.id} projectId={projectId} profile={active} openDeploy={() => openDeploy(active)} edit={() => setEditor({ draft: active, existing: true })} copy={copy} remove={() => {
      if (!window.confirm(`Delete the "${active.name}" profile? This only removes its saved configuration; it does not delete files or cloud services.`)) return;
      const saved = updateWorkspace((workspace) => ({ ...workspace, projects: workspace.projects.map((item) => item.id === projectId ? { ...item, services: (item.services ?? []).filter((profile) => profile.id !== active.id) } : item) }));
      notify(saved ? "Service profile removed." : "Profile removed for this session only. Browser storage unavailable.");
    }}/>}</div> : <section className="panel service-empty"><span className="notes-empty-icon"><Icon name={scopeIcons[scope]} size={28}/></span><h2>{query.trim() ? "No matching services" : `Set up ${scope === "other" ? "another service" : `your ${scope}`}`}</h2><p>{query.trim() ? "Try another search, or add a new profile." : "Start with a suggested configuration, review it and save your profile."}</p><button className="button" onClick={start}>{query.trim() ? "Add service" : "Review suggested profile"}<Icon name="arrow" size={15}/></button><span className="service-empty-hint">Use Project discovery above for detected suggestions.</span></section>}
    {editor && <ServiceEditor key={editor.draft.id} projectId={projectId} initial={editor.draft} existing={editor.existing} onClose={() => setEditor(null)} notify={notify} onSaved={(profile) => { setEditor(null); onSelect(profile.id); onSaved(profile); requestAnimationFrame(() => workspaceRef.current?.querySelector<HTMLButtonElement>(".service-record-actions .button")?.focus({ preventScroll: true })); }}/>}
  </div>;
}

