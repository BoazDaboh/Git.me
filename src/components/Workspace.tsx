"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import Icon from "./Icon";
import WindowHistory from "./WindowHistory";
import ThemeToggle from "./ThemeToggle";
import NetworkStatus from "./NetworkStatus";
import Notes from "./Notes";
import Lessons from "./Lessons";
import ToolsLibrary from "./ToolsLibrary";
import CvBuilder from "./CvBuilder";
import PostActivityPicker from "./PostActivityPicker";
import PdfWorkspace from "./PdfWorkspace";
import ActivityCalendar from "./ActivityCalendar";
import TaskBoard from "./TaskBoard";
import ManagedRuns from "./ManagedRuns";
import ApiTester from "./ApiTester";
import ProjectChecks from "./ProjectChecks";
import DebugServiceSetup from "./DebugServiceSetup";
import GitHubConnectionPanel from "./GitHubConnection";
import VercelConnectionPanel from "./VercelConnection";
import Mark from "./ProjectMark";
import ProjectForm from "./ProjectForm";
import ProjectPin from "./ProjectPin";
import ProjectDetails from "./ProjectDetails";
import ProjectDirectorySettings from "./ProjectDirectorySettings";
import WorkspaceNameSettings from "./WorkspaceNameSettings";
import { CompanionBadge, CompanionChip, CompanionPanel, useCompanion } from "./CompanionConnection";
import WorkspaceStorage, { StorageLabel, StorageNotice } from "./WorkspaceStorage";
import { useWorkspace, updateWorkspace } from "@/lib/store";
import { clock, downloadText, navigation, projectHref, type Section, type Task } from "@/lib/workspace";

const titles: Record<Section, string> = { overview: "Overview", projects: "Projects", flow: "Tasks & flow", debug: "Debug & tests", notes: "Notes & plans", lessons: "Lessons", tools: "Tools library", publishing: "Portfolio & posts", reports: "Reports", pdf: "PDF workspace", settings: "Settings" };

const href = (id: Section) => id === "overview" ? "/" : `/${id}`;

function Badge({ children, tone = "neutral" }: { children: ReactNode; tone?: string }) { return <span className={`badge ${tone}`}><i />{children}</span>; }
function Empty({ children }: { children: ReactNode }) { return <div className="empty"><Icon name="search" size={26}/><p>{children}</p></div>; }
function Dialog({ title, children, onClose, drawer = false }: { title: string; children: ReactNode; onClose: () => void; drawer?: boolean }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => { const dialog = ref.current; dialog?.showModal(); return () => dialog?.close(); }, []);
  return <dialog ref={ref} className={drawer ? "dialog drawer" : "dialog"} onCancel={onClose} onClick={(event) => { if (event.target === ref.current) onClose(); }} aria-labelledby="dialog-title"><div className="dialog-heading"><h2 id="dialog-title">{title}</h2><button className="icon-button" aria-label="Close dialog" onClick={onClose}><Icon name="close"/></button></div>{children}</dialog>;
}

export default function Workspace({ section, projectId, createService = false }: { section: Section; projectId?: string; createService?:boolean }) {
  const router = useRouter();
  const data = useWorkspace();
  const isWorking = !!data.focus;
  const { connection } = useCompanion();
  const [query, setQuery] = useState("");
  const [menu, setMenu] = useState(false);
  const [filter, setFilter] = useState("All projects");
  const [tab, setTab] = useState("Portfolio");
  const [modal, setModal] = useState<{ kind: "project" | "task"; id?: string; status?: Task["status"] } | null>(null);
  const [toast, setToast] = useState("");
  const [now, setNow] = useState(0);
  const [focusProject, setFocusProject] = useState("");
  const [debugProject, setDebugProject] = useState("");
  const [debugTab, setDebugTab] = useState("Local services");
  const [draft, setDraft] = useState("");
  const [editingDraft, setEditingDraft] = useState<string | null>(null);
  const [draftDrawer, setDraftDrawer] = useState(false);
  const [aiContextOpen, setAiContextOpen] = useState(false);
  const [aiContextReady, setAiContextReady] = useState(false);
  const [draftPlatform, setDraftPlatform] = useState("LinkedIn");
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => { const tick = () => setNow(Date.now()); const first = window.setTimeout(tick, 0); const timer = window.setInterval(tick, isWorking ? 1000 : 60000); return () => { clearTimeout(first); clearInterval(timer); }; }, [isWorking]);
  useEffect(() => { if (!toast) return; const timer = setTimeout(() => setToast(""), 4500); return () => clearTimeout(timer); }, [toast]);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if ((event.ctrlKey || event.metaKey) && event.key === "k") { event.preventDefault(); searchRef.current?.focus(); } if (event.key === "Escape") setMenu(false); };
    window.addEventListener("keydown", onKey); return () => window.removeEventListener("keydown", onKey);
  }, []);

  function save(update: Parameters<typeof updateWorkspace>[0], message?: string) {
    const persisted = updateWorkspace(update);
    if (!persisted) setToast("Browser storage unavailable. Changes last for this session only.");
    else if (message) setToast(message);
  }
  function toggleFocus(working: boolean) {
    if (working && !data.focus) save((value) => ({ ...value, focus: { project: "", started: Date.now() } }), "Work session started.");
    if (!working && data.focus) save((value) => ({ ...value, sessions: [...value.sessions, { project: value.focus!.project, started: value.focus!.started, ended: Math.max(value.focus!.started, timerNow) }], focus: null }), "Work session saved.");
  }
  const timerNow = connection.state === "connected" ? Math.min(now, (connection.checkedAt ?? now) + 15000) : connection.lastConnectedAt ?? data.focus?.started ?? now;
  const elapsed = data.focus && now ? ((data.focus.elapsedMs ?? 0) + Math.max(0, timerNow - data.focus.started)) / 1000 : 0;
  const needle = query.toLowerCase().trim();
  const orderedProjects = [...data.projects].sort((a, b) => Number(!!b.pinned) - Number(!!a.pinned));
  const projects = orderedProjects.filter((project) => `${project.name} ${project.description} ${project.stack.join(" ")}`.toLowerCase().includes(needle) && (filter === "All projects" || project.stage === filter));
  const tasks = data.tasks.filter((task) => `${task.title} ${data.projects.find((p) => p.id === task.project)?.name}`.toLowerCase().includes(needle));
  const activeProject = data.projects.find((project) => project.id === projectId);
  const editing = data.projects.find((project) => project.id === modal?.id);
  const selectedDebug = data.projects.find((project) => project.id === debugProject) ?? data.projects[0];
  const done = data.tasks.filter((task) => task.status === "Done").length;
  const inProgress = data.tasks.filter((task) => task.status === "In progress").length;

  async function copy(text: string) {
    try { await navigator.clipboard.writeText(text); setToast("Copied to clipboard."); }
    catch { setToast("Clipboard unavailable. Select and copy the text manually."); }
  }
  function submitTask(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = new FormData(event.currentTarget); const title = String(form.get("title") ?? "").trim(); if (!title) return;
    save((value) => ({ ...value, tasks: [...value.tasks, { id: crypto.randomUUID(), title, project: String(form.get("project")), priority: String(form.get("priority")), status: modal?.status ?? "Planned" }] }), "Task added."); setModal(null);
  }
  function report() {
    const week = data.sessions.filter((session) => session.ended >= Date.now() - 7 * 86400000);
    const total = week.reduce((sum, session) => sum + session.ended - session.started, 0);
    return `# Git/Me workspace report\n\nGenerated ${new Date().toLocaleDateString("en-GB")}\n\nBased on your local workspace records. \n\n## Focus: last 7 days\n${week.length} completed sessions, ${clock(total / 1000)} total.\n\n## Current projects\n${data.projects.map((p) => `- ${p.name}: ${p.stage}`).join("\n")}\n\n## Current task snapshot\n${data.tasks.map((t) => `- [${t.status === "Done" ? "x" : " "}] ${t.title} (${t.status})`).join("\n")}\n`;
  }

  function projectTable(compact = false) {
    return <div className="table-scroll"><table><thead><tr><th>Project</th><th>Stage</th>{!compact && <th>Stack</th>}<th>Workspace</th><th><span className="sr-only">Actions</span></th></tr></thead><tbody>{projects.map((project) => <tr key={project.id} className="project-list-row">
      <td><Link className="project-name" href={projectHref(project.id)} aria-label={`Manage ${project.name}`}><Mark project={project}/><span><strong>{project.name}</strong><small>{project.description}</small></span></Link></td>
      <td><Badge tone={project.stage === "Live" ? "green" : project.stage === "Building" ? "orange" : "neutral"}>{project.stage}</Badge></td>
      {!compact && <td><div className="tags">{project.stack.map((tech) => <span key={tech}>{tech}</span>)}</div></td>}
      <td><span className="branch"><Icon name="branch" size={14}/>{project.branch}</span></td>
      <td className="project-open-cell"><ProjectPin project={project}/><Link href={projectHref(project.id)} className="icon-button" aria-label={`Open ${project.name}`}><Icon name="chevron" size={17}/></Link></td>
    </tr>)}</tbody></table>{projects.length === 0 && <Empty>No projects match your search.</Empty>}</div>;
  }
  function taskRow(task: Task) {
    const project = data.projects.find((project) => project.id === task.project);
    return <div className="task-row" key={task.id}><button className={`task-check ${task.status === "Done" ? "checked" : ""}`} aria-label={`${task.status === "Done" ? "Reopen" : "Complete"} ${task.title}`} aria-pressed={task.status === "Done"} onClick={() => save((value) => ({ ...value, tasks: value.tasks.map((t) => t.id === task.id ? { ...t, status: t.status === "Done" ? "Planned" : "Done" } : t) }))}>{task.status === "Done" && <Icon name="check" size={13}/>}</button><div><strong className={task.status === "Done" ? "completed" : ""}>{task.title}</strong><small>{project?.name ?? "Unassigned"}</small></div>{task.priority === "High" && <span className="priority">High</span>}</div>;
  }

  return <div className="workspace">
    <a className="skip-link" href="#main-content">Skip to content</a>
    {menu && <button className="sidebar-backdrop" aria-label="Close navigation" onClick={() => setMenu(false)}/>}
    <aside id="sidebar" className={`sidebar ${menu ? "open" : ""}`}>
      <Link href="/" className="brand" aria-label="Git Me overview">git<span>/</span>me<span className="brand-dot">.</span><span className="brand-label">PERSONAL</span></Link>
      <NetworkStatus/>
      <nav aria-label="Main navigation">{navigation.map((item, index) => <div key={item.id}>{index === 0 || item.group !== navigation[index - 1].group ? <div className="nav-label">{item.group}</div> : null}<div className={`nav-entry ${item.id === "notes" ? "nav-entry-with-window" : ""}`}><Link href={href(item.id)} onClick={() => setMenu(false)} className={`nav-item ${section === item.id ? "active" : ""}`} aria-current={section === item.id ? "page" : undefined}><Icon name={item.icon}/><span>{item.label}</span>{item.id === "projects" && <span className="nav-count">{data.projects.length}</span>}</Link>{item.id === "notes" && <button type="button" className="icon-button notes-window-button" aria-label="Open Notes and Plans in a new window" title="Open in a new window" onClick={() => { const popup = window.open("/notes-window", "gitme-notes", "popup,width=1100,height=800,resizable=yes,scrollbars=yes"); if (!popup) setToast("Allow pop-ups to open Notes & Plans in a new window."); else popup.focus(); }}><Icon name="external" size={15}/></button>}</div></div>)}</nav>
      <div className="sidebar-bottom"><Link href="/settings" className={`nav-item ${section === "settings" ? "active" : ""}`}><Icon name="settings"/><span>Settings</span></Link><div className="profile"><span className="avatar">{(data.workspaceName?.trim() || "My workspace").charAt(0).toUpperCase()}</span><div><strong>{data.workspaceName?.trim() || "My workspace"}</strong><small>Personal account</small></div><span className="profile-dot"/></div></div>
    </aside>
    <div className="main-column"><header className="topbar"><div className="breadcrumb"><WindowHistory/><button className="icon-button menu-button" aria-label="Open navigation" aria-expanded={menu} aria-controls="sidebar" onClick={() => setMenu(!menu)}><Icon name="menu"/></button><span>Workspace</span><span className="slash">/</span>{projectId ? <><Link href="/projects">Projects</Link><span className="slash">/</span><strong>{activeProject?.name ?? "Project"}</strong></> : <strong>{titles[section]}</strong>}</div><div className="topbar-right"><ThemeToggle/><CompanionChip/><div className="mode-switch" aria-label="Focus mode"><button onClick={() => toggleFocus(true)} aria-pressed={!!data.focus} className={data.focus ? "on work" : ""}><span className="tiny-dot"/>Work</button><button onClick={() => toggleFocus(false)} aria-pressed={!data.focus} className={!data.focus ? "on" : ""}>Away</button></div></div></header>
    <main id="main-content"><div className="page-heading"><div><div className="project-title-row">{projectId && activeProject && <Mark project={activeProject} large/>}<h1>{projectId ? activeProject?.name ?? "Project" : titles[section]}<span>.</span></h1></div>{projectId && activeProject?.description && <p>{activeProject.description}</p>}</div><div className="heading-actions">{section === "overview" && !projectId && <div className="activity-timer"><span>Today work time</span><strong>{clock(elapsed)}</strong><button aria-label={data.focus ? "Stop timer" : "Start timer"} title={data.focus ? "Stop timer" : "Start timer"} onClick={() => toggleFocus(!data.focus)}><Icon name={data.focus ? "pause" : "play"} size={17}/></button></div>}{projectId && activeProject && <ProjectPin project={activeProject}/ >}{projectId ? (activeProject ? <button className="button primary" onClick={() => setModal({ kind: "project", id: projectId })}>Edit project<Icon name="arrow" size={15}/></button> : <Link href="/projects" className="button">All projects</Link>) : section === "overview" || section === "projects" ? <button className="button primary" onClick={() => setModal({ kind: "project" })}><Icon name="plus" size={16}/>New project</button> : section === "flow" ? <button className="button primary" onClick={() => setModal({ kind: "task" })}><Icon name="plus" size={16}/>Add task</button> : section === "reports" ? <button className="button primary" onClick={() => downloadText("gitme-report.md", report(), "text/markdown")}><Icon name="download" size={16}/>Export report</button> : null}</div></div>

    {!["settings", "pdf", "reports"].includes(section) && <div className="toolbar"><label className="search-field"><Icon name="search" size={17}/><input ref={searchRef} type="search" aria-label={`Search ${titles[section].toLowerCase()}`} placeholder={projectId ? "Search this project..." : section === "tools" ? "Find a tool or resource..." : "Search your workspace..."} value={query} onChange={(e) => setQuery(e.target.value)}/><kbd>Ctrl K</kbd></label><span className="toolbar-note"><span className="tiny-dot"/><StorageLabel/></span></div>}

    <StorageNotice/>

    {projectId && <ProjectDetails key={projectId} projectId={projectId} createService={createService} query={query} clearSearch={() => setQuery("")} edit={() => setModal({ kind: "project", id: projectId })} addTask={(status) => setModal({ kind: "task", status })} notify={setToast} copy={copy}/>}

    {section === "overview" && <>
      <ActivityCalendar now={now} focusNow={timerNow}/>
      <div className="stats-grid">{[{ label: "Total projects", value: data.projects.length, icon: "folder", text: "Across your workspace" }, { label: "In development", value: data.projects.filter((p) => p.stage === "Building").length, icon: "terminal", text: "Your current builds" }, { label: "Open tasks", value: data.tasks.length - done, icon: "flow", text: `${inProgress} in progress` }, { label: "Completed tasks", value: done, icon: "check", text: "In this workspace" }].map((stat) => <div className="stat-card" key={stat.label}><div><span>{stat.label}</span><Icon name={stat.icon} size={17}/></div><strong>{String(stat.value).padStart(2, "0")}</strong><small>{stat.text}</small></div>)}</div>
      <section className="panel project-panel"><div className="panel-heading"><h2>Your projects <span>{data.projects.length}</span></h2><Link href="/projects" className="text-link">View all<Icon name="arrow" size={15}/></Link></div>{projectTable(true)}</section>
      <div className="bottom-grid"><section className="panel"><div className="panel-heading"><h2>Up next</h2><Link href="/flow" className="text-link">Open board<Icon name="arrow" size={15}/></Link></div><div className="task-list">{tasks.filter((task) => task.status !== "Done").slice(0, 3).map(taskRow)}{tasks.filter((task) => task.status !== "Done").length === 0 && <Empty>You are all caught up.</Empty>}</div></section><section className="panel quick-panel"><div className="panel-heading"><h2>Within reach</h2><span className="muted">Resources</span></div><div className="quick-links">{[{ label: "API workspace", icon: "terminal", path: "/debug", hint: "Checks & requests" }, { label: "Your toolbox", icon: "tools", path: "/tools", hint: "Saved resources" }, { label: "Weekly report", icon: "chart", path: "/reports", hint: "Review your work" }].map((item) => <Link href={item.path} key={item.path}><span className="quick-icon"><Icon name={item.icon}/></span><div><strong>{item.label}</strong><small>{item.hint}</small></div><Icon name="chevron" size={15}/></Link>)}</div></section></div>
    </>}

    {section === "projects" && !projectId && <><div className="section-controls"><div className="filter-tabs">{["All projects", "Building", "Planning", "Live"].map((value) => <button key={value} className={filter === value ? "selected" : ""} onClick={() => setFilter(value)}>{value}</button>)}</div><span className="muted">{projects.length} projects</span></div><section className="panel">{projectTable()}</section></>}

    {section === "flow" && <><div className="section-controls"><span className="section-label"><Icon name="flow" size={17}/>Project board</span><span className="muted">{data.tasks.length} tasks</span></div><TaskBoard tasks={tasks} query={query} onAddTask={(status) => setModal({ kind: "task", status })} notify={setToast}/><section className="panel flow-panel"><div className="panel-heading"><h2>Project lifecycle</h2><span className="muted">Starting template</span></div><div className="lifecycle">{["Idea", "Planning", "Building", "Review", "Live", "Maintain"].map((step, index) => <div key={step}><span>{String(index + 1).padStart(2, "0")}</span><strong>{step}</strong>{index < 5 && <Icon name="arrow" size={17}/>}</div>)}</div></section></>}

    {section === "debug" && <><div className="section-controls"><select aria-label="Debug project" value={debugTab !== "Local services" ? selectedDebug?.id || "" : debugProject} onChange={(e) => setDebugProject(e.target.value)}>{debugTab === "Local services" && <option value="">All projects</option>}{orderedProjects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select><CompanionBadge/></div><DebugServiceSetup projects={debugTab === "Local services" && !debugProject ? orderedProjects : selectedDebug ? [selectedDebug] : []}/><div className="filter-tabs standalone">{["Local services", "Checks", "API requests"].map((value) => <button key={value} className={debugTab === value ? "selected" : ""} onClick={() => setDebugTab(value)}>{value}</button>)}</div>{debugTab === "Local services" ? <ManagedRuns projectId={debugProject}/> : debugTab === "Checks" ? selectedDebug ? <ProjectChecks key={`${selectedDebug.id}:${selectedDebug.folder}`} project={selectedDebug} copy={copy}/> : <Empty>Add a project to detect its checks.</Empty> : selectedDebug ? <ApiTester key={selectedDebug.id} project={selectedDebug}/> : <Empty>Add a project to test its API.</Empty>}</>}

    {section === "lessons" && <Lessons query={query} notify={setToast}/> }

    {section === "notes" && <Notes query={query} notify={setToast} clearSearch={() => setQuery("")}/>}

    {section === "tools" && <ToolsLibrary query={query} notify={setToast}/>}

    {section === "publishing" && <><div className="publishing-header"><div className="filter-tabs standalone">{["Portfolio", "Posts", "CV"].map((value) => <button key={value} className={tab === value ? "selected" : ""} aria-pressed={tab === value} onClick={() => setTab(value)}>{value}</button>)}</div>{tab === "Posts" && <button className="button" onClick={() => setDraftDrawer(true)}><Icon name="file" size={16}/>Drafts <span>{data.drafts.length}</span></button>}</div>{tab === "Posts" ? <><div className="post-composer-layout"><section className="panel composer"><div className="panel-heading"><h2>{editingDraft ? "Edit draft" : "Write an update"}</h2><Badge>Draft</Badge></div>{editingDraft && <div className="editing-draft-id"><span>Draft ID: <code>{editingDraft}</code></span><button className="icon-button" type="button" aria-label="Copy draft ID" title="Copy draft ID" onClick={() => copy(editingDraft)}><Icon name="copy" size={15}/></button><details className="draft-id-help"><summary aria-label="About draft IDs" title="About draft IDs">i</summary><div>Give this ID to your connected MCP client to identify the draft when updating it.</div></details></div>}<form onSubmit={(event) => { event.preventDefault(); if (!draft.trim()) return; save((value) => ({ ...value, drafts: editingDraft ? value.drafts.map(item => item.id === editingDraft ? { ...item, platform: draftPlatform, project: focusProject, text: draft.trim() } : item) : [{ id: crypto.randomUUID(), platform: draftPlatform, project: focusProject, text: draft.trim() }, ...value.drafts] }), "Draft saved. Nothing has been published."); setDraft(""); setEditingDraft(null); }}><div className="form-row"><fieldset className="post-platforms"><legend>Platform</legend><div>{["LinkedIn", "X", "Other"].map(platform => <button type="button" key={platform} className={`post-platform ${draftPlatform === platform ? "selected" : ""}`} aria-label={platform} title={platform} aria-pressed={draftPlatform === platform} onClick={() => setDraftPlatform(platform)}>{platform === "LinkedIn" ? <span className="linkedin-mark">in</span> : platform === "X" ? <span className="x-mark">𝕏</span> : <Icon name="globe" size={19}/>}</button>)}</div></fieldset><label>Project<select value={focusProject} onChange={(e) => setFocusProject(e.target.value)}>{orderedProjects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label></div><label className="sr-only" htmlFor="post-text">Post text</label><textarea id="post-text" placeholder="What did you build or learn?" value={draft} onChange={(e) => setDraft(e.target.value)} required maxLength={5000}/><div className="composer-footer"><span className="muted">{draft.length} characters</span><button className="button" type="button" onClick={() => { setAiContextReady(false); setAiContextOpen(true); }}><Icon name="spark" size={15}/>Generate post</button>{editingDraft && <button className="button" type="button" onClick={() => { if (window.confirm("Discard changes in the editor?")) { setDraft(""); setEditingDraft(null); } }}>Cancel</button>}<button className="button primary" type="submit">{editingDraft ? "Update draft" : "Save draft"}<Icon name="arrow" size={15}/></button></div></form></section></div></> : tab === "Portfolio" ? <div className="tools-grid">{projects.map((project) => <article className="panel portfolio-card" key={project.id}><ProjectPin project={project}/><Mark project={project}/><h2>{project.name}</h2><p>{project.description}</p><div className="tags">{project.stack.map((tech) => <span key={tech}>{tech}</span>)}</div><div className="portfolio-status"><span>Portfolio status</span><Badge>Needs review</Badge></div><button className="button" onClick={() => setModal({ kind: "project", id: project.id })}>Edit project details<Icon name="arrow" size={15}/></button></article>)}</div> : <CvBuilder notify={setToast}/>}</>}

    {section === "reports" && <><div className="stats-grid three"><div className="stat-card"><div><span>Focus sessions</span><Icon name="clock"/></div><strong>{data.sessions.filter((s) => s.ended >= now - 7 * 86400000).length}</strong><small>Completed in the last 7 days</small></div><div className="stat-card"><div><span>Completed tasks</span><Icon name="check"/></div><strong>{done}</strong><small>Current workspace snapshot</small></div><div className="stat-card"><div><span>Saved drafts</span><Icon name="send"/></div><strong>{data.drafts.length}</strong><small>Ready for your next review</small></div></div><div className="reports-grid"><section className="panel"><div className="panel-heading"><h2>Your focus history</h2><span className="muted">Manual sessions</span></div>{data.sessions.length ? <div className="session-list">{[...data.sessions].reverse().map((session, index) => <div className="session-row" key={`${session.started}-${index}`}><span className="quick-icon"><Icon name="clock"/></span><div><strong>{data.projects.find((p) => p.id === session.project)?.name ?? (session.project ? "Project unavailable" : "General work")}</strong><small>{new Date(session.started).toLocaleString("en-GB")}</small></div><code>{clock((session.ended - session.started) / 1000)}</code></div>)}</div> : <div className="focus-empty"><Icon name="clock" size={30}/><h3>Your first session starts here</h3><p>Use Work mode to record time.</p><Link href="/" className="button">Go to overview<Icon name="arrow" size={15}/></Link></div>}</section><section className="panel report-card"><div className="panel-heading"><h2>Weekly digest</h2><Icon name="mail"/></div><p>Projects, task status and your last seven days of focus sessions.</p><div className="digest-line"><Icon name="check" size={16}/>Preview and export locally</div><div className="digest-line muted"><Icon name="mail" size={16}/>Email delivery not connected</div><button className="button primary" onClick={() => downloadText("gitme-report.md", report(), "text/markdown")}>Download summary<Icon name="download" size={15}/></button></section></div></>}

    {section === "pdf" && <PdfWorkspace notify={setToast}/>}

    {section === "settings" && <div className="settings-grid"><CompanionPanel notify={setToast}/><GitHubConnectionPanel/><VercelConnectionPanel/><section className="panel settings-panel"><div className="panel-heading"><h2>Workspace</h2><Icon name="settings"/></div><WorkspaceNameSettings notify={setToast}/><ProjectDirectorySettings notify={setToast}/><div className="setting-row"><div><strong>Storage</strong><small>SQLite through the Go companion</small></div><StorageLabel/></div><div className="setting-row"><div><strong>Cloud accounts</strong><small>GitHub and Vercel connections above.</small></div><Icon name="branch"/></div><div className="setting-row"><div><strong>Install Git/Me</strong><small>Use your browser&apos;s Install app option when available. The local server must remain running.</small></div><Icon name="monitor"/></div></section><WorkspaceStorage notify={setToast}/></div>}
    </main><footer className="workspace-footer"><span>git<span>/</span>me.</span><span>Personal developer workspace</span><span>INTERFACE / 0.1</span></footer></div>

    {aiContextOpen && <Dialog title="Generate post" onClose={() => setAiContextOpen(false)}>{aiContextReady ? <div className="ai-connect-step"><span className="badge">Next step</span><h2>Connect an AI client</h2><p>Connect an MCP client to Git/Me’s local server, then ask it to read your saved context and write a draft.</p><p>Wait for the workspace to show Saved to SQLite before continuing.</p><p>ChatGPT requires a separate connection setup. The Generate button does not call an AI model yet.</p><div className="ai-connect-actions"><button className="button" onClick={() => setAiContextReady(false)}>Back</button><button className="button primary" onClick={() => setAiContextOpen(false)}>Done</button></div></div> : <><p className="ai-context-intro">Choose a repository and save context for your AI client.</p><PostActivityPicker projects={data.projects} onSelect={(project, text, platform) => { setFocusProject(project); setDraftPlatform(platform); setDraft(current => current.trim() ? `${current.trimEnd()}\n\n${text}` : text); setAiContextReady(true); }}/></>}</Dialog>}
    {draftDrawer && <Dialog title="Saved drafts" drawer onClose={() => setDraftDrawer(false)}><section className="drafts drawer-drafts">{data.drafts.filter((post) => post.text.toLowerCase().includes(needle)).map((post) => <article className="panel draft-card" key={post.id}><div><span className="mini-tag">{post.platform}</span><span className="muted">{data.projects.find((p) => p.id === post.project)?.name}</span></div><p>{post.text}</p><div className="draft-identifier"><span>ID: <code>{post.id}</code></span><button className="icon-button" aria-label={`Copy draft ID ${post.id}`} title="Copy draft ID" onClick={() => copy(post.id)}><Icon name="copy" size={15}/></button></div><div className="draft-actions"><button className="icon-button" aria-label="Delete draft" title="Delete draft" onClick={() => { if (!window.confirm("Delete this draft?")) return; save(value => ({ ...value, drafts: value.drafts.filter(item => item.id !== post.id) }), "Draft deleted."); if (editingDraft === post.id) { setEditingDraft(null); setDraft(""); } }}><Icon name="trash" size={16}/></button><button className="text-link" onClick={() => { if (draft.trim() && !window.confirm("Replace the text currently in the editor?")) return; setDraft(post.text); setDraftPlatform(post.platform); setFocusProject(post.project); setEditingDraft(post.id); setDraftDrawer(false); }}>Edit</button><button className="text-link" onClick={() => copy(post.text)}><Icon name="copy" size={14}/>Copy post</button><span className="muted">Not published</span></div></article>)}</section></Dialog>}
    {modal && <Dialog title={modal.kind === "project" ? editing ? "Edit project" : "New project" : "New task"} onClose={() => setModal(null)}>{modal.kind === "project" ? <ProjectForm key={editing?.id ?? "new"} project={editing} notify={setToast} onCancel={() => setModal(null)} onSaved={(project, created) => { setModal(null); if (created) router.push(projectHref(project.id)); }}/> : <form className="editor-form" onSubmit={submitTask}><label>Task<input name="title" required maxLength={180} autoFocus placeholder="What needs to happen?"/></label><div className="form-row"><label>Project<select name="project" defaultValue={projectId ?? data.projects[0]?.id}>{orderedProjects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label><label>Priority<select name="priority"><option>Normal</option><option>High</option></select></label></div><div className="dialog-footer"><button className="button" type="button" onClick={() => setModal(null)}>Cancel</button><button className="button primary" type="submit">Add task<Icon name="plus" size={15}/></button></div></form>}</Dialog>}

    {toast && <div className="toast" role="status"><Icon name="check" size={17}/>{toast}<button className="icon-button" aria-label="Dismiss notification" onClick={() => setToast("")}><Icon name="close" size={15}/></button></div>}
  </div>;
}




