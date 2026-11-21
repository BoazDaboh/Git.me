"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useWorkspace, useWorkspaceStorage, updateWorkspace } from "@/lib/store";
import { vercelRequest, projectURL, deploymentURL, type VercelDeployment as ProviderDeployment, type VercelActivity, type VercelConnection, type VercelLink, type VercelProject, type VercelProjectPage, type VercelTeam, type VercelTeamPage } from "@/lib/vercel";
import { useVercel } from "./VercelConnection";
import Icon from "./Icon";
import { adoptSavedDeployment, serviceRepository } from "@/lib/service-deployment-link";

export default function VercelDeployment({ projectId, serviceId, mode="connection", onSetup }: { projectId: string; serviceId: string; mode?:"connection"|"status";onSetup?:()=>void }) {
  const workspace = useWorkspace(); const cloud = useVercel();
  const project = workspace.projects.find((item) => item.id === projectId);
  if (!project) return null;
  const service = project.services?.find((item) => item.id === serviceId);
  if (!service) return null;
  const link = service.vercel;
  const identity = `${cloud.available}:${cloud.connections.map((item) => `${item.id}:${item.connected}:${item.checked_at}`).join(",")}`;
  return <DeploymentBody key={JSON.stringify([projectId,serviceId,link,project.vercel,identity,mode])} projectId={projectId} serviceId={serviceId} link={link} savedLink={project.vercel} repository={serviceRepository(service)} mode={mode} onSetup={onSetup}/>;
}

function DeploymentBody({ projectId, serviceId, link, savedLink, repository, mode, onSetup }: { projectId: string; serviceId: string; link?: VercelLink; savedLink?: VercelLink; repository: string;mode:"connection"|"status";onSetup?:()=>void }) {
  const cloud = useVercel(); const storage = useWorkspaceStorage();
  const linkedAccount = cloud.connections.find((item) => item.id === link?.connectionId);
  const accounts = cloud.available ? cloud.connections.filter((item) => item.connected) : [];
  const [accountId, setAccountId] = useState(() => linkedAccount?.connected ? linkedAccount.id : accounts[0]?.id || "");
  const [picker, setPicker] = useState(false);
  const [activity, setActivity] = useState<VercelActivity | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [reusing,setReusing] = useState(false);
  const reuseRequest = useRef<AbortController | null>(null);
  useEffect(() => () => reuseRequest.current?.abort(),[]);
  const savedAccount = cloud.connections.find((item) => item.id === savedLink?.connectionId);
  const refresh = useRef<(force?: boolean) => Promise<void>>(async () => {});
  const canRead = cloud.available && !!linkedAccount?.connected && storage.state === "saved";
  useEffect(() => {
    if (!link || !canRead || mode !== "status") return;
    let active = true, busy = false; let controller: AbortController | null = null;
    const check = async (force = false) => {
      if (!active || busy) return;
      busy = true; controller = new AbortController(); setLoading(true);
      const params = new URLSearchParams({ project_id: projectId, ...(serviceId ? { service_id: serviceId } : {}), ...(force ? { refresh: "1" } : {}) });
      try { const result = await vercelRequest<VercelActivity>(`activity?${params}`, undefined, controller.signal); if (active) { setActivity(result); setError(""); } }
      catch (error) { if (active) setError(error instanceof Error ? error.message : "Deployment status unavailable."); }
      finally { busy = false; if (active) setLoading(false); }
    };
    refresh.current = check;
    const poll = () => { if (document.visibilityState === "visible") void check(); };
    void check(); const timer = setInterval(poll, 60000);
    window.addEventListener("focus", poll); document.addEventListener("visibilitychange", poll);
    return () => { active = false; controller?.abort(); clearInterval(timer); window.removeEventListener("focus", poll); document.removeEventListener("visibilitychange", poll); };
  }, [projectId, serviceId, link, canRead, mode]);
  function save(value?: VercelLink) {
    updateWorkspace((workspace) => ({ ...workspace, projects: workspace.projects.map((item) => item.id !== projectId ? item : { ...item, services: (item.services ?? []).map((profile) => profile.id === serviceId ? { ...profile, vercel: value } : profile) }) }));
    setPicker(false);
  }
  async function reuseSavedLink() {
    if (!savedLink || !savedAccount?.connected || reusing || !repository || storage.state !== "saved") return;
    const controller = new AbortController();reuseRequest.current = controller;setReusing(true);setError("");
    try {
      const params = new URLSearchParams({connection_id:savedLink.connectionId,team_id:savedLink.teamId,name:savedLink.projectId});
      const verified = await vercelRequest<VercelProject>(`project?${params}`,undefined,controller.signal);
      if (controller.signal.aborted) return;
      if (verified.repository?.toLowerCase() !== repository.toLowerCase()) throw new Error("The saved Vercel project must use this service's GitHub repository. Choose the matching project instead.");
      const persisted = updateWorkspace((data) => adoptSavedDeployment(data,projectId,serviceId,savedLink,repository,verified));
      if (!persisted) setError("Link saved for this session only. Browser storage is unavailable.");
    } catch(error) {if (!controller.signal.aborted) setError(error instanceof Error ? error.message : "Could not reuse the saved deployment link.");}
    finally {if (!controller.signal.aborted) setReusing(false);}
  }
  if (mode === "status") return <section className="panel vercel-deployment" aria-label="Live deployment status"><div className="panel-heading"><h2><Icon name="globe" size={16}/>Live status</h2><div className="github-actions"><span className="badge">Vercel</span><button className="button integration-action" disabled={!canRead || loading} onClick={() => {void refresh.current(true);}}><Icon name="refresh" size={14}/>{loading ? "Refreshing..." : "Refresh status"}</button></div></div><div className="vercel-deployment-body">{link && <p className="git-help">{link.scopeSlug}/{link.projectName}</p>}{activity ? <div className="vercel-production"><span>Current production</span><strong>{activity.production ? <DeploymentState state={activity.production.state}/> : "No production deployment reported"}</strong>{activity.production && <small>{formatDate(activity.production.created_at)}</small>}{activity.production && deploymentURL(activity.production.url) && <a className="text-link" href={deploymentURL(activity.production.url)} target="_blank" rel="noreferrer">Open production<Icon name="external" size={13}/></a>}<small>Checked {new Date(activity.checked_at).toLocaleTimeString("en-GB")}{error ? " · Last successful reading" : ""}</small></div> : <p className="git-help">{loading ? "Reading Vercel status..." : !link ? "Connect a deployment provider in Setup." : !linkedAccount?.connected ? "Reconnect the linked Vercel account in Settings." : "Status has not been read."}</p>}{activity && <RecentDeployments deployments={activity.deployments} warnings={activity.warnings}/>}{error && <p className="github-error" role="alert">{error}</p>}<button className="text-link" onClick={onSetup}><Icon name="link" size={13}/>Manage deployment connection</button></div></section>;
  return <section id={`service-deploy-${serviceId}`} className="panel vercel-deployment" aria-label="Service deployment connection">
    <div className="panel-heading"><h2><Icon name="globe" size={16}/>Hosting connection</h2><span className="badge"><i/>Vercel</span></div>
    <div className="vercel-deployment-body">
      <div className="github-repository-heading"><div><h3>{activity?.project.name || link?.projectName || "No Vercel project linked"}</h3>{link && <p>{linkedAccount ? `@${linkedAccount.username}` : "Account disconnected"} · {link.scopeSlug}</p>}{activity && <small>Last checked {new Date(activity.checked_at).toLocaleTimeString("en-GB")}{error ? " · Last successful reading" : ""}</small>}</div>
        <div className="github-actions">{link && <><button className="button integration-action integration-destructive" disabled={storage.state !== "saved"} onClick={() => { if (window.confirm("Remove this Vercel link? The hosted project will stay unchanged.")) save(); }}><Icon name="close" size={15}/>Unlink</button></>}
          <button className={picker ? "button integration-action" : "button primary integration-action"} disabled={accounts.length === 0 || storage.state !== "saved"} onClick={() => setPicker((value) => !value)}><Icon name={picker ? "close" : "link"} size={16}/>{picker ? "Close picker" : link ? "Change Vercel project" : "Link Vercel project"}</button>
        </div>
      </div>
      {accounts.length === 0 && <p className="github-hint"><Link className="button integration-secondary" href="/settings#vercel-connection"><Icon name="link" size={16}/>Connect Vercel in Settings<Icon name="arrow" size={13}/></Link>{cloud.error && <span>{cloud.error}</span>}</p>}
      {link && !linkedAccount?.connected && <p className="github-hint">The linked account is unavailable. Reconnect it or choose another account explicitly.</p>}
      {storage.state !== "saved" && <p className="github-hint">Wait for Saved to SQLite before linking or refreshing.</p>}
      {!link && savedLink && <div className="saved-deployment-link"><div><strong>{savedLink.scopeSlug}/{savedLink.projectName}</strong><p>Saved project link. Reuse it for this service after checking the GitHub repository.</p></div><button className="button integration-secondary" disabled={reusing || storage.state !== "saved" || !savedAccount?.connected || !repository} onClick={() => {void reuseSavedLink();}}><Icon name="link" size={15}/>{reusing ? "Verifying..." : "Use saved deployment link"}</button>{!savedAccount?.connected && <p>Reconnect the original Vercel account in Settings to reuse this link.</p>}{!repository && <p>Set this service&apos;s matching GitHub repository first.</p>}</div>}
      {picker && accounts.length > 0 && <div className="github-picker vercel-picker"><label>Vercel account<select aria-label="Vercel account" value={accountId} onChange={(event) => setAccountId(event.target.value)}>{accounts.map((item) => <option key={item.id} value={item.id}>@{item.username}</option>)}</select></label><ProjectPicker key={accountId} account={accounts.find((item) => item.id === accountId)!} initial={link?.connectionId === accountId ? link : undefined} onSave={save}/></div>}
      {error && <p className="github-error" role="alert">{error}</p>}{loading && !activity && <p className="github-hint" role="status">Reading Vercel deployments...</p>}
      {link && <a className="button integration-secondary vercel-dashboard-link" href={projectURL(link.scopeSlug, activity?.project.name || link.projectName)} target="_blank" rel="noreferrer"><Icon name="link" size={16}/>Open Vercel<Icon name="external" size={13}/></a>}
    </div>
  </section>;
}

function ProjectPicker({ account, initial, onSave }: { account: VercelConnection; initial?: VercelLink; onSave: (link: VercelLink) => void }) {
  const [teams, setTeams] = useState<VercelTeam[]>([]);
  const [next, setNext] = useState<number | string | null>(null);
  const [teamId, setTeamId] = useState(initial?.teamId || "");
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController(); request.current = controller; let active = true;
    void vercelRequest<VercelTeamPage>(`teams?connection_id=${encodeURIComponent(account.id)}`, undefined, controller.signal).then((result) => {
      if (!active) return;
      setTeams(result.teams); setNext(result.pagination.next); setTeamId(initial?.teamId ?? result.teams[0]?.id ?? ""); setReady(true);
    }).catch((error) => { if (active) setError(error instanceof Error ? error.message : "Could not list Vercel teams."); });
    return () => { active = false; request.current?.abort(); };
  }, [account.id, initial?.teamId]);
  async function more() {
    setBusy(true); setError(""); const controller = new AbortController(); request.current = controller;
    try { const params = new URLSearchParams({ connection_id: account.id, ...(typeof next === "string" ? { from: next } : { until: String(next || 0) }) }); const result = await vercelRequest<VercelTeamPage>(`teams?${params}`, undefined, controller.signal); setTeams((items) => [...items, ...result.teams.filter((team) => !items.some((item) => item.id === team.id))]); setNext(result.pagination.next); }
    catch (error) { setError(error instanceof Error ? error.message : "Could not list more teams."); }
    finally { setBusy(false); }
  }
  const selectedTeam = teams.find((team) => team.id === teamId);
  return <div className="vercel-project-picker">
    {error && <p className="github-error" role="alert">{error}</p>}
    {!ready && !error && <p className="github-hint" role="status">Reading Vercel teams...</p>}
    {ready && <><label>Team or personal scope<select aria-label="Team or personal scope" value={teamId} onChange={(event) => setTeamId(event.target.value)}><option value="">Personal scope (@{account.username})</option>{teamId && !selectedTeam && <option value={teamId}>Saved team ({teamId})</option>}{teams.map((team) => <option key={team.id} value={team.id}>{team.name || team.slug}</option>)}</select></label>{next && <button className="text-link" disabled={busy} onClick={() => { void more(); }}>{busy ? "Loading teams..." : "Load more teams"}</button>}
      {teamId && !selectedTeam ? <p className="github-hint">Load more teams or choose an available scope.</p> : <ProjectList key={`${account.id}:${teamId}`} account={account} team={selectedTeam} onSave={onSave}/>}</>}
  </div>;
}

function ProjectList({ account, team, onSave }: { account: VercelConnection; team?: VercelTeam; onSave: (link: VercelLink) => void }) {
  const storage = useWorkspaceStorage();
  const [page, setPage] = useState<VercelProjectPage | null>(null);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<VercelProject | null>(null);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);
  const version = useRef(0);
  const run = useRef<(cursor?: number | string) => Promise<void>>(async () => {});
  useEffect(() => {
    let active = true;
    const load = async (cursor: number | string = 0, search = "") => {
      request.current?.abort(); const current = ++version.current; const controller = new AbortController(); request.current = controller;
      setBusy(true); setError(""); setSelected(null); setPage(null);
      const params = new URLSearchParams({ connection_id: account.id, team_id: team?.id || "", ...(typeof cursor === "string" ? { from: cursor } : { until: String(cursor) }), search });
      try { const result = await vercelRequest<VercelProjectPage>(`projects?${params}`, undefined, controller.signal); if (active && current === version.current) setPage(result); }
      catch (error) { if (active && current === version.current) setError(error instanceof Error ? error.message : "Could not list Vercel projects."); }
      finally { if (active && current === version.current) setBusy(false); }
    };
    run.current = (until = 0) => load(until, query);
    const timer = setTimeout(() => { void load(0, query); }, 350);
    return () => { active = false; clearTimeout(timer); request.current?.abort(); };
  }, [account.id, team?.id, query]);
  return <div className="vercel-project-list">
    <label>Search Vercel projects<input value={query} onChange={(event) => setQuery(event.target.value)} maxLength={100} placeholder="Project name"/></label>
    {error && <p className="github-error" role="alert">{error}</p>}
    <div className="github-repo-list" aria-label="Available Vercel projects">{page?.projects.map((project) => <button key={project.id} disabled={busy} aria-pressed={selected?.id === project.id} className={selected?.id === project.id ? "selected" : ""} onClick={() => setSelected(project)}><strong>{project.name}</strong><span>{project.framework || "Other framework"}</span></button>)}{busy && <p role="status">Reading Vercel projects...</p>}{page && !busy && page.projects.length === 0 && <p>No projects in this scope.</p>}</div>
    <div className="github-pagination"><button className="text-link" disabled={busy} onClick={() => { void run.current(); }}>First page</button><button className="text-link" disabled={busy || !page?.pagination.next} onClick={() => { void run.current(page?.pagination.next || 0); }}>Next page</button></div>
    {selected && <div className="github-selected"><div><strong>{selected.name}</strong><p>{team?.name || `@${account.username}`}</p></div><button className="button primary integration-action" disabled={busy || storage.state !== "saved"} onClick={() => onSave({ connectionId: account.id, projectId: selected.id, projectName: selected.name, teamId: team?.id || "", scopeSlug: team?.slug || account.username })}><Icon name="link" size={16}/>Link selected project</button></div>}
  </div>;
}

function RecentDeployments({deployments,warnings}:{deployments:ProviderDeployment[];warnings:string[]}) {return <details className="vercel-history"><summary>Recent provider builds</summary>{deployments.map((deployment) => <div className="vercel-deployment-row" key={deployment.id}><div><DeploymentState state={deployment.state}/><span className="vercel-target">{deployment.target}</span><small>{formatDate(deployment.created_at)}</small>{deployment.sha && <code>{deployment.branch} · {deployment.sha.slice(0,7)}</code>}</div>{deploymentURL(deployment.url) && <a className="text-link" href={deploymentURL(deployment.url)} target="_blank" rel="noreferrer">Open deployment<Icon name="external" size={13}/></a>}</div>)}{!deployments.length && <p className="git-help">{warnings.length ? "Build history unavailable." : "No builds reported."}</p>}{warnings.length > 0 && <ul className="github-warnings">{warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>}</details>;}

function DeploymentState({ state }: { state: string }) { return <span className="vercel-state" data-state={state}>{state.toLowerCase().replaceAll("_", " ")}</span>; }
function formatDate(value: number) { return value > 0 ? new Date(value).toLocaleString("en-GB") : "Time unavailable"; }

