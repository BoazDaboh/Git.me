"use client";

import { useEffect, useRef, useState } from "react";
import { processRequest, type EndpointHealth, type Run } from "@/lib/processes";
import { useWorkspaceStorage } from "@/lib/store";
import { serviceScope, type ServiceProfile, type ServiceScope } from "@/lib/services";
import type { Project } from "@/lib/workspace";
import GitCommand from "./GitCommand";
import Icon from "./Icon";

const labels = { frontend: "Frontend", backend: "Backend", other: "Other services" };
const healthRefreshMs = 30 * 60 * 1000;
const processLabels: Record<Run["state"],string> = {not_managed:"Not managed",running:"Running",stopping:"Stopping",stopped:"Stopped",failed:"Failed",exited:"Exited"};

export default function ProjectStartupOverview({ project, openScope, openService, openDeploy }: { project: Project; openScope: (scope: ServiceScope) => void; openService: (profile: ServiceProfile) => void; openDeploy: () => void }) {
  const [states,setStates] = useState<Record<string,Run["state"]>>({});
  const [available,setAvailable] = useState(false);
  const [error,setError] = useState("");
  const refreshProcesses = useRef<() => Promise<void>>(async () => {});
  useEffect(() => {
    let active = true, busy = false, due = 0; let controller: AbortController | null = null;
    const check = async () => {
      if (!active || busy) return;
      busy = true;due = Date.now()+healthRefreshMs; controller = new AbortController();
      try {
        const runs = await processRequest<Run[]>("/services/runs",undefined,controller.signal);
        if (active) { setStates(Object.fromEntries(runs.filter((run) => run.project_id === project.id).map((run) => [run.service_id,run.state])));setAvailable(true);setError(""); }
      } catch (error) { if (active) { setAvailable(false);setError(error instanceof Error ? error.message : "Process status unavailable."); } }
      finally { busy = false; }
    };
    refreshProcesses.current = check;
    const wake = () => { if (document.visibilityState === "visible" && Date.now() >= due) void check(); };
    const first = setTimeout(wake,0); const timer = setInterval(wake,60000);
    window.addEventListener("focus",wake);window.addEventListener("online",wake);window.addEventListener("gitme:companion-ready",wake);document.addEventListener("visibilitychange",wake);
    return () => { active = false;controller?.abort();refreshProcesses.current = async () => {};clearTimeout(first);clearInterval(timer);window.removeEventListener("focus",wake);window.removeEventListener("online",wake);window.removeEventListener("gitme:companion-ready",wake);document.removeEventListener("visibilitychange",wake); };
  },[project.id]);
  return <div className="setup-overview-grid">{(["frontend","backend","other"] as ServiceScope[]).map((scope) => {
    const profiles = (project.services || []).filter((profile) => serviceScope(profile.role) === scope);
    return <section className="panel setup-overview-card" key={scope}><div className="setup-card-heading"><h2>{labels[scope]}</h2><button className="text-link" onClick={() => openScope(scope)}>Open<Icon name="arrow" size={14}/></button></div>{profiles.length ? profiles.map((profile) => <ServiceStartup key={JSON.stringify([profile.id,profile.folder,profile.port,profile.healthPath,profile.environments.local.url])} projectId={project.id} profile={profile} processState={available ? states[profile.id] || "not_managed" : null} processError={error} refreshProcesses={() => { void refreshProcesses.current(); }} open={() => openService(profile)}/>) : <p>No profiles added.</p>}</section>;
  })}<section className="panel setup-overview-card"><div className="setup-card-heading"><h2>Deployment</h2><button className="text-link" onClick={openDeploy}>Open<Icon name="arrow" size={14}/></button></div><div className="setup-profile"><span>GitHub: {(project.services || []).filter((profile) => profile.github).length} linked services</span><span>Deploy: {(project.services || []).filter((profile) => profile.vercel).length} linked services</span>{project.vercel && <small>Saved project link available for a service.</small>}<small>Saved connections. Open deployment to refresh status.</small></div></section></div>;
}

function localEndpoint(profile: ServiceProfile): string {
  const raw = profile.environments.local.url || (profile.port && ["frontend","backend"].includes(profile.role) ? `http://127.0.0.1:${profile.port}` : "");
  try { const url = new URL(raw); if (["http:","https:"].includes(url.protocol) && !url.username && !url.password && ["localhost","127.0.0.1","[::1]","::1"].includes(url.hostname.toLowerCase())) return url.href; } catch {}
  return "";
}
function commandInFolder(folder: string, command: string): string {
  if (!command.trim()) return "";
  return folder && !/["%!\r\n\0]/.test(folder) ? `cd /d "${folder}" && ${command}` : command;
}
function ServiceStartup({ projectId, profile, processState, processError, refreshProcesses, open }: { projectId: string; profile: ServiceProfile; processState: Run["state"] | null; processError: string; refreshProcesses: () => void; open: () => void }) {
  const storage = useWorkspaceStorage();
  const card = useRef<HTMLElement>(null);
  const [health,setHealth] = useState<EndpointHealth | null>(null);
  const [healthError,setHealthError] = useState("");
  const [checking,setChecking] = useState(false);
  const [checkedAt,setCheckedAt] = useState<number | null>(null);
  const refresh = useRef<() => Promise<void>>(async () => {});
  const nextHealthCheck = useRef(0);
  const endpoint = localEndpoint(profile);
  useEffect(() => {
    if (storage.state !== "saved" || !endpoint) return;
    let active = true, busy = false; let controller: AbortController | null = null;
    const check = async () => {
      if (!active || busy) return;
      busy = true;nextHealthCheck.current = Date.now()+healthRefreshMs;controller = new AbortController();setChecking(true);
      try {
        const result = await processRequest<EndpointHealth>("/services/health",{project_id:projectId,service_id:profile.id},controller.signal);
        if (active) { setHealth(result);setHealthError("");setCheckedAt(Date.now()); }
      } catch (error) { if (active) { setHealthError(error instanceof Error ? error.message : "Health check unavailable.");setCheckedAt(Date.now()); } }
      finally { busy = false;if (active) setChecking(false); }
    };
    refresh.current = check;
    const poll = () => { const bounds = card.current?.getBoundingClientRect(); if (document.visibilityState === "visible" && bounds && bounds.top < innerHeight && bounds.bottom > 0 && Date.now() >= nextHealthCheck.current) void check(); };
    const wake = poll;
    const first = setTimeout(poll,0);const timer = setInterval(poll,60000);
    window.addEventListener("focus",wake);window.addEventListener("online",wake);window.addEventListener("gitme:companion-ready",wake);document.addEventListener("visibilitychange",wake);
    return () => { active = false;if (busy) nextHealthCheck.current = 0;controller?.abort();clearTimeout(first);clearInterval(timer);refresh.current = async () => {};window.removeEventListener("focus",wake);window.removeEventListener("online",wake);window.removeEventListener("gitme:companion-ready",wake);document.removeEventListener("visibilitychange",wake); };
  },[projectId,profile.id,storage.state,endpoint]);
  const healthState = !endpoint ? "unconfigured" : healthError ? "unknown" : !health ? "unchecked" : health.responding ? "healthy" : health.status ? "error" : "unreachable";
  const healthLabel = !endpoint ? "Not configured" : healthError ? "Unavailable" : !health ? checking ? "Checking..." : "Not checked" : health.responding ? `Healthy · HTTP ${health.status}` : health.status ? `HTTP ${health.status}` : "Unreachable";
  return <article className="setup-profile overview-startup" ref={card} aria-label={`Startup ${profile.name}`}><div className="overview-startup-heading"><strong>{profile.name}</strong><button className="text-link" onClick={open}>Open service<Icon name="arrow" size={13}/></button></div><span>{profile.runtime || profile.stack.join(", ") || "Runtime not set"}</span><small>GitHub: {profile.github?.fullName || "Not linked"} · Deploy: {profile.vercel?.projectName || profile.deployment.provider || "Not linked"}</small>
    <div className="overview-startup-folder"><code>{profile.folder || "Service folder not set"}</code></div>
    <GitCommand title="Start app · Command Prompt" command={commandInFolder(profile.folder,profile.commands.develop)} description="Run in Command Prompt from this folder and keep the terminal open." empty="Set a Development command in this service's profile."/>
    {(profile.commands.install || profile.commands.build || profile.commands.start) && <details className="overview-extra-commands"><summary>Setup & production commands</summary>{([['install','Install dependencies'],['build','Build'],['start','Start built app']] as const).map(([key,label]) => profile.commands[key] ? <GitCommand key={key} title={label} command={commandInFolder(profile.folder,profile.commands[key])} description="Run in Command Prompt from this service's folder."/> : null)}</details>}
    <div className="overview-service-health"><small>Auto refresh · 30 min</small><div aria-live="polite"><span>Process: <strong>{processState ? processLabels[processState] : processError ? "Unavailable" : "Checking..."}</strong></span><span className="overview-health-badge" data-state={healthState}>HTTP: {healthLabel}</span></div><div className="overview-health-actions"><button className="text-link" disabled={checking || !endpoint || storage.state !== "saved"} onClick={() => { refreshProcesses();void refresh.current(); }}><Icon name="refresh" size={13}/>{checking ? "Checking..." : "Check health"}</button>{endpoint && <a className="text-link" href={endpoint} target="_blank" rel="noreferrer">Open app<Icon name="external" size={13}/></a>}</div>{checkedAt && <small>Checked {new Date(checkedAt).toLocaleTimeString("en-GB")}{health && !healthError ? ` · ${health.duration_ms} ms` : ""}</small>}{health && !healthError && <code>{health.url}</code>}{!endpoint && <small>Set a local URL or HTTP port to check health.</small>}{(healthError || processError) && <small className="overview-health-error">{healthError || processError}</small>}</div>
  </article>;
}
