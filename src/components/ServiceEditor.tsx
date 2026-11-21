"use client";

import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { useWorkspace, updateWorkspace } from "@/lib/store";
import { stackCatalog } from "@/lib/workspace";
import { environmentNames, serviceCommands, serviceRoles, validateService, type ServiceProfile, type ServiceEnvironmentName, type ServiceRole } from "@/lib/services";
import Icon from "./Icon";
import StackPicker from "./StackPicker";
import { commandTemplates, editServiceCommand, refreshServiceSuggestions, suggestServiceCommands, type CommandTemplate } from "@/lib/service-commands";

const panels = ["Identity", "Local setup", "Local environment"];

export default function ServiceEditor({ initial, projectId, existing, onClose, onSaved, notify }: { initial: ServiceProfile; projectId: string; existing: boolean; onClose: () => void; onSaved: (service: ServiceProfile) => void; notify: (message: string) => void }) {
  const data = useWorkspace();
  const [draft, setDraft] = useState(() => refreshServiceSuggestions(initial));
  const [panel, setPanel] = useState("Identity");
  const [port, setPort] = useState(initial.port?.toString() ?? "");
  const [stackQuery, setStackQuery] = useState("");
  const [variables, setVariables] = useState<Record<ServiceEnvironmentName, string>>({ local: initial.environments.local.variables.join(", "), preview: initial.environments.preview.variables.join(", "), production: initial.environments.production.variables.join(", ") });
  const [error, setError] = useState("");
  const dialog = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const suggestion = suggestServiceCommands(draft);
  useEffect(() => { const node = dialog.current; node?.showModal(); return () => node?.close(); }, []);
  function field<K extends keyof ServiceProfile>(key: K, value: ServiceProfile[K]) { setDraft((current) => ({ ...current, [key]: value })); }
  function changePanel(next: string) {
    const pending = stackQuery.trim();
    if (pending) {
      setDraft((current) => current.stack.some((item) => item.toLowerCase() === pending.toLowerCase()) ? current : refreshServiceSuggestions({ ...current, stack: [...current.stack, stackCatalog.find((item) => item.toLowerCase() === pending.toLowerCase()) ?? pending].slice(0, 20) }));
      setStackQuery("");
    }
    setPanel(next);
  }
  function changeUrl(name: ServiceEnvironmentName, url: string) { setDraft((current) => ({ ...current, environments: { ...current.environments, [name]: { ...current.environments[name], url } } })); }
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    let prepared = draft;
    const pending = stackQuery.trim();
    if (pending && !draft.stack.some((item) => item.toLowerCase() === pending.toLowerCase())) prepared = refreshServiceSuggestions({ ...draft, stack: [...draft.stack, stackCatalog.find((item) => item.toLowerCase() === pending.toLowerCase()) ?? pending] });
    const clean = { ...prepared, name: prepared.name.trim(), folder: prepared.folder.trim(), repository: prepared.repository.trim(), branch: prepared.branch.trim(), runtime: prepared.runtime.trim(), healthPath: prepared.healthPath.trim() };
    const invalid = validateService(clean, port.trim(), variables);
    if (invalid) { setPanel(invalid.panel === "Environments" ? "Local environment" : invalid.panel); setError(invalid.message); return; }
    const stack = [...clean.stack];
    const service: ServiceProfile = { ...clean, stack: stack.slice(0, 20), port: port.trim() ? Number(port) : null, updatedAt: Date.now(), environments: Object.fromEntries(environmentNames.map(({ value }) => [value, { url: clean.environments[value].url.trim(), variables: [...new Set(variables[value].split(/[\s,]+/).filter(Boolean))] }])) as ServiceProfile["environments"] };
    const currentProject = data.projects.find((project) => project.id === projectId);
    if (!currentProject) { setError("This project is no longer available."); return; }
    const saved = updateWorkspace((workspace) => ({ ...workspace, projects: workspace.projects.map((project) => project.id !== projectId ? project : { ...project, services: (project.services ?? []).some((item) => item.id === service.id) ? (project.services ?? []).map((item) => item.id === service.id ? { ...item, name:service.name,role:service.role,stack:service.stack,runtime:service.runtime,folder:service.folder,repository:service.repository,branch:service.branch,port:service.port,healthPath:service.healthPath,commands:service.commands,commandTemplate:service.commandTemplate,commandOverrides:service.commandOverrides,updatedAt:service.updatedAt,environments:{...item.environments,local:service.environments.local} } : item) : [...(project.services ?? []), service] }) }));
    if (!saved) { setError("Profile changed for this session only. Browser storage is unavailable; export your workspace to keep a copy."); return; }
    notify(existing ? "Service profile updated." : "Service profile saved."); onSaved(service);
  }
  return <dialog ref={dialog} className="dialog service-dialog" aria-labelledby={titleId} onCancel={onClose} onClick={(event) => { if (event.target === dialog.current) onClose(); }}>
    <div className="dialog-heading"><div><h2 id={titleId}>{existing ? "Edit service" : "New service"}</h2><p>{initial.detection ? "Detected configuration · Review before saving" : "Manual configuration"}</p></div><button className="icon-button" aria-label="Close service editor" onClick={onClose}><Icon name="close"/></button></div>
    <form className="service-editor-form" noValidate onSubmit={submit}>
      <div className="service-form-tabs" aria-label="Service configuration sections">{panels.map((item, index) => <button key={item} type="button" aria-label={item} aria-pressed={panel === item} className={panel === item ? "selected" : ""} onClick={() => changePanel(item)}><span>{index + 1}</span>{item}</button>)}</div>
      <div className="service-form-content">
      {panel === "Identity" && <>
        <div className="form-row"><label>Service title<input value={draft.name} maxLength={80} onChange={(event) => field("name", event.target.value)} placeholder="Test, Production, Review..." autoFocus/></label><label>Service type<select value={draft.role} onChange={(event) => setDraft((current) => refreshServiceSuggestions({ ...current, role: event.target.value as ServiceRole }))}>{serviceRoles.map((role) => <option value={role.value} key={role.value}>{role.label}</option>)}</select></label></div>
        <label>Runtime<input list={`${titleId}-runtimes`} value={draft.runtime} maxLength={80} placeholder="Node.js, Go, PHP..." onChange={(event) => field("runtime", event.target.value)} onBlur={() => { if (!draft.stack.length && (draft.commandTemplate ?? "auto") === "auto") setDraft((current) => refreshServiceSuggestions(current)); }}/></label><datalist id={`${titleId}-runtimes`}>{["Browser", "Node.js", "Go", "PHP", "Python", "Bun", "PostgreSQL", "MySQL", "Redis"].map((value) => <option key={value} value={value}/>)}</datalist>
        <StackPicker selected={draft.stack} onChange={(stack) => setDraft((current) => refreshServiceSuggestions({ ...current, stack }))} query={stackQuery} onQueryChange={setStackQuery} catalog={[...new Set([...stackCatalog, ...data.projects.flatMap((project) => project.stack)])]}/>
        <label>Command suggestions<select value={draft.commandTemplate ?? "auto"} onChange={(event) => setDraft((current) => refreshServiceSuggestions({ ...current, commandTemplate: event.target.value as CommandTemplate }))}>{commandTemplates.map((template) => <option value={template.value} key={template.value}>{template.label}</option>)}</select></label><p className="service-field-help">{initial.detection ? "Commands came from a project scan. Rescan after changing scripts; template suggestions can replace detected commands." : "Suggestions follow this selection. Project files have not been scanned."}</p>
        <label>Repository<input value={draft.repository} maxLength={500} placeholder="https://github.com/username/repository" onChange={(event) => field("repository", event.target.value)}/></label>
        <label>Branch<input value={draft.branch} maxLength={100} placeholder="main" onChange={(event) => field("branch", event.target.value)}/></label>
      </>}
      {panel === "Local setup" && <>
        <label>Service folder<input value={draft.folder} maxLength={500} placeholder="C:\Users\USER\project\my-project\web" onChange={(event) => field("folder", event.target.value)}/></label>
        <div className="form-row"><label>Local port<input type="text" inputMode="numeric" value={port} maxLength={5} placeholder="3000" onChange={(event) => {
          const next = event.target.value; setPort(next); setDraft((current) => refreshServiceSuggestions({ ...current, port: /^\d+$/.test(next) && Number(next) >= 1 && Number(next) <= 65535 ? Number(next) : null }));
          const current = draft.environments.local.url;
          if (!current || /^http:\/\/(localhost|127\.0\.0\.1):\d+\/?$/.test(current)) { const host = current.includes("127.0.0.1") ? "127.0.0.1" : "localhost"; changeUrl("local", next ? `http://${host}:${next}` : ""); }
        }}/></label><label>Health path<input value={draft.healthPath} maxLength={200} placeholder="/health" onChange={(event) => field("healthPath", event.target.value)}/></label></div>
        <label>Local URL<input value={draft.environments.local.url} maxLength={500} placeholder="http://localhost:3000" onChange={(event) => changeUrl("local", event.target.value)}/></label>
        <div className="service-command-suggestions"><div className="service-form-section-title"><Icon name="terminal" size={16}/><h3>{suggestion.title}</h3></div><button type="button" className="text-link" disabled={suggestion.template === "custom"} onClick={() => setDraft((current) => refreshServiceSuggestions(current, true))}>Apply suggestions</button></div><p className="service-field-help">{suggestion.hint}</p><p className="service-field-help">Custom fields stay as you entered them. Apply suggestions replaces them. Nothing runs from this form.</p>
        {serviceCommands.map((command) => <label key={command.value}><span className="service-command-label">{command.label} command{draft.commandOverrides?.includes(command.value) && <span>Custom</span>}</span><input aria-label={`${command.label} command`} className="command-input" value={draft.commands[command.value]} maxLength={1000} onChange={(event) => setDraft((current) => editServiceCommand(current, command.value, event.target.value))} placeholder={command.value === "install" ? suggestion.installHint ?? "Optional" : "Optional"}/></label>)}
      </>}
      {panel === "Local environment" && <><p className="service-field-help">Save URLs and required variable names. Keep secret values out of these fields.</p>{environmentNames.filter(({value}) => value === "local").map(({ value, label }) => <fieldset className="service-environment-fields" key={value}><legend>{label}</legend><label>{label} URL<input value={draft.environments[value].url} maxLength={500} placeholder={value === "local" ? "http://localhost:3000" : "https://..."} onChange={(event) => changeUrl(value, event.target.value)}/></label><label>{label} variable names<textarea value={variables[value]} maxLength={6500} placeholder="API_URL, PORT" rows={2} onChange={(event) => setVariables((current) => ({ ...current, [value]: event.target.value }))}/></label></fieldset>)}</>}
      </div>
      {error && <p className="project-form-error service-form-error" role="alert">{error}</p>}
      <div className="service-form-footer"><button className="button" type="button" onClick={onClose}>Cancel</button><div>{panel !== panels[panels.length - 1] && <button className="button" type="button" onClick={() => changePanel(panels[panels.indexOf(panel) + 1])}>Next<Icon name="arrow" size={14}/></button>}<button className="button primary" type="submit">Save profile<Icon name="check" size={15}/></button></div></div>
    </form>
  </dialog>;
}
