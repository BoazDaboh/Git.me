"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, useSyncExternalStore } from "react";
import { useWorkspace, updateWorkspace, useWorkspaceStorage } from "@/lib/store";
import { projectHref, type Project, type Task } from "@/lib/workspace";
import Icon from "./Icon";
import Notes from "./Notes";
import TaskBoard from "./TaskBoard";
import ProjectServices from "./ProjectServices";
import ServiceEditor from "./ServiceEditor";
import ProjectScan from "./ProjectScan";
import ProjectStartupOverview from "./ProjectStartupOverview";
import ProjectDeployments from "./ProjectDeployments";
import { createServiceDraft, serviceScope, type ServiceProfile, type ServiceScope } from "@/lib/services";

const subscribe = () => () => {};

export default function ProjectDetails({ projectId, createService = false, query, clearSearch, edit, addTask, notify, copy }: { projectId: string; createService?:boolean; query: string; clearSearch: () => void; edit: () => void; addTask: (status: Task["status"]) => void; notify: (text: string) => void; copy: (text: string) => void }) {
  const data = useWorkspace();
  const router = useRouter();
  const storage = useWorkspaceStorage();
  const [creatingService,setCreatingService] = useState(createService);
  const hydrated = useSyncExternalStore(subscribe, () => true, () => false);
  const [storedTab, setTab] = useState("Overview");
  const tab = storedTab === "GitHub" ? "Overview" : storedTab === "Vercel" ? "Deploy" : storedTab;
  const [brief, setBrief] = useState<string | null>(null);
  const [selectedService, setSelectedService] = useState<string | null>(null);
  const project = data.projects.find((item) => item.id === projectId);
  if (!project) return <section className="panel project-missing"><Icon name="folder" size={30}/><h2>{hydrated ? "Project not found in this workspace" : "Loading project..."}</h2>{hydrated && <><p>Your projects are currently stored on this device.</p><Link className="button" href="/projects">Back to projects<Icon name="arrow" size={15}/></Link></>}</section>;
  const tasks = data.tasks.filter((task) => task.project === projectId);
  const linkedNotes = data.notes.filter((note) => note.project === projectId);
  const tabs = ["Overview", "Frontend", "Backend", "Other services", "Deploy", "Tasks", "Notes & plans", "Details"];
  const profileTabs: Record<ServiceScope, string> = { frontend: "Frontend", backend: "Backend", other: "Other services" };
  const profileScope: ServiceScope | null = tab === "Frontend" ? "frontend" : tab === "Backend" ? "backend" : tab === "Other services" ? "other" : null;
  const safeLink = (value: string) => /^https?:\/\//i.test(value);

  function briefPanel() {
    return <section className="panel project-brief"><div className="panel-heading"><h2>Project brief</h2><Icon name="notes" size={17}/></div><label className="sr-only" htmlFor="project-brief">Project brief</label><textarea id="project-brief" maxLength={10000} value={brief ?? project!.note} onChange={(event) => setBrief(event.target.value)} placeholder="Goals, context and next steps..."/><div className="project-panel-footer"><span className="muted">Local workspace</span><button className="button" onClick={() => {
      const saved = updateWorkspace((value) => ({ ...value, projects: value.projects.map((item) => item.id === projectId ? { ...item, note: brief ?? project!.note } : item) }));
      notify(saved ? "Project brief saved." : "Brief changed for this session only. Browser storage unavailable.");
    }}>Save brief<Icon name="check" size={15}/></button></div></section>;
  }

  function locations() {
    return <section className="panel project-locations"><div className="panel-heading"><h2>Locations</h2><button className="text-link" onClick={edit}>Edit<Icon name="arrow" size={14}/></button></div><div className="project-location-row"><span className="quick-icon"><Icon name="folder"/></span><div><small>Local folder</small><code>{project!.folder || "Not set"}</code></div>{project!.folder && <button className="icon-button" aria-label="Copy local folder" onClick={() => copy(project!.folder)}><Icon name="copy" size={15}/></button>}</div>{[{ label: "Local URL", url: project!.local, icon: "monitor" }, { label: "Live URL", url: project!.url, icon: "globe" }].map((item) => <div className="project-location-row" key={item.label}><span className="quick-icon"><Icon name={item.icon}/></span><div><small>{item.label}</small>{safeLink(item.url) ? <a href={item.url} target="_blank" rel="noreferrer">{item.url}</a> : <span className="muted">Not set</span>}</div>{safeLink(item.url) && <a className="icon-button" href={item.url} target="_blank" rel="noreferrer" aria-label={`Open ${item.label.toLowerCase()}`}><Icon name="external" size={14}/></a>}</div>)}</section>;
  }

  const closeService = () => {setCreatingService(false);router.replace(projectHref(projectId),{scroll:false});};
  return <div className="project-page">{creatingService && hydrated && storage.state !== "loading" && <NewServiceProfile project={project} notify={notify} close={closeService} saved={(profile) => {setSelectedService(profile.id);setTab(profileTabs[serviceScope(profile.role)]);clearSearch();closeService();}}/>}<div className="project-page-navigation"><Link href="/projects" className="text-link"><Icon name="chevron" size={13} style={{ transform: "rotate(180deg)" }}/>All projects</Link><div className="project-page-stack"><span className="badge orange"><i/>{project.stage}</span><div className="tags">{project.stack.map((item) => <span key={item}>{item}</span>)}</div></div></div>
    <ProjectScan key={`${project.id}:${project.folder}`} project={project} edit={edit} notify={notify} openProfile={(profile) => { setTab(profileTabs[serviceScope(profile.role)]); setSelectedService(profile.id); clearSearch(); }}/>
    <div className="project-tabs" aria-label="Project sections">{tabs.map((value) => <button key={value} className={tab === value ? "selected" : ""} aria-pressed={tab === value} onClick={() => { setTab(value); clearSearch(); }}>{value}{value === "Tasks" && <span>{tasks.length}</span>}{value === "Notes & plans" && <span>{linkedNotes.length}</span>}</button>)}</div>
    {tab === "Overview" && <><ProjectStartupOverview project={project} openScope={(scope) => { setTab(profileTabs[scope]);clearSearch(); }} openService={(profile) => { setSelectedService(profile.id);setTab(profileTabs[serviceScope(profile.role)]);clearSearch(); }} openDeploy={() => setTab("Deploy")}/><div className="project-overview-grid">{briefPanel()}{locations()}</div><section className="panel project-next-tasks"><div className="panel-heading"><h2>Next tasks</h2><button className="text-link" onClick={() => setTab("Tasks")}>Open tasks<Icon name="arrow" size={14}/></button></div>{tasks.filter(task => task.status !== "Done").slice(0, 4).map(task => <div className="project-next-task" key={task.id}><Icon name="flow" size={15}/><span>{task.title}</span><span className="badge">{task.status}</span></div>)}{!tasks.some(task => task.status !== "Done") && <div className="project-tasks-empty"><span>No open tasks.</span><button className="text-link" onClick={() => addTask("Planned")}><Icon name="plus" size={14}/>Add task</button></div>}</section></>}
    {profileScope && <ProjectServices projectId={projectId} scope={profileScope} openDeploy={(profile) => {setSelectedService(profile.id);setTab("Deploy");clearSearch();}} query={query} selectedId={selectedService} onSelect={setSelectedService} notify={notify} copy={copy} onSaved={(profile) => { setTab(profileTabs[serviceScope(profile.role)]); clearSearch(); }}/>} 
    {tab === "Deploy" && <ProjectDeployments project={project} selectedId={selectedService} onSelect={setSelectedService} notify={notify} openService={(profile) => {setSelectedService(profile.id);setTab(profileTabs[serviceScope(profile.role)]);clearSearch();}} addService={() => {setTab("Frontend");clearSearch();}}/>}
    {tab === "Tasks" && <><div className="section-controls"><span className="section-label">{tasks.length} tasks</span><button className="button" onClick={() => addTask("Planned")}><Icon name="plus" size={15}/>Add task</button></div><TaskBoard projectId={projectId} query={query} tasks={tasks.filter((task) => task.title.toLowerCase().includes(query.toLowerCase().trim()))} onAddTask={addTask} notify={notify}/></>}
    {tab === "Notes & plans" && <Notes projectId={projectId} query={query} clearSearch={clearSearch} notify={notify}/>}
    {tab === "Details" && <><section className="panel project-data-panel"><div className="panel-heading"><h2>Project details</h2><button className="button" onClick={edit}>Edit project<Icon name="arrow" size={15}/></button></div><dl><div><dt>Name</dt><dd>{project.name}</dd></div><div><dt>Description</dt><dd>{project.description || "Not set"}</dd></div><div><dt>Stage</dt><dd>{project.stage}</dd></div><div><dt>Git branch</dt><dd><code>{project.branch}</code></dd></div><div><dt>Progress</dt><dd>{project.progress}% · Manual</dd></div><div><dt>Stack</dt><dd>{project.stack.join(", ") || "Not set"}</dd></div></dl></section><div className="project-overview-grid project-details-bottom">{locations()}{briefPanel()}</div></>}
  </div>;
}

function NewServiceProfile({project,notify,close,saved}:{project:Project;notify:(message:string)=>void;close:()=>void;saved:(profile:ServiceProfile)=>void}) {
  const [draft]=useState(()=>{
    const backend=project.stack.some(item=>["go","golang","php","composer","laravel","node.js","nestjs","express","python","django","fastapi"].includes(item.toLowerCase()));
    return createServiceDraft(project,backend?"backend":"frontend");
  });
  return <ServiceEditor initial={draft} projectId={project.id} existing={false} onClose={close} onSaved={saved} notify={notify}/>;
}
