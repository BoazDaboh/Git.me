"use client";

import { useEffect, useState } from "react";
import { serviceScope, type ServiceProfile } from "@/lib/services";
import { updateWorkspace, useWorkspaceStorage } from "@/lib/store";
import type { Project } from "@/lib/workspace";
import type { ReleaseRoute } from "@/lib/releases";
import type { DeploymentAction } from "@/lib/manual-deployment";
import Icon from "./Icon";
import VercelDeployment from "./VercelDeployment";
import GitHubRepository from "./GitHubRepository";
import DeploymentSettings from "./DeploymentSettings";
import LocalGit from "./LocalGit";
import LocalUpload from "./LocalUpload";
import DeploymentTracker from "./DeploymentTracker";
import DeploymentRequestHistory from "./DeploymentRequestHistory";

const groups = [{scope:"frontend",label:"Frontend"},{scope:"backend",label:"Backend"},{scope:"other",label:"Other services"}] as const;
const flows = [
  {id:"github",title:"Push through GitHub",description:"Stage, commit, push and follow your host's deployment.",icon:"branch"},
  {id:"vercel-remote",title:"Existing GitHub commit",description:"Deploy or rebuild code already on GitHub.",icon:"globe"},
  {id:"vercel-local",title:"Local upload",description:"Upload current local files without pushing to GitHub.",icon:"folder"},
] as const;

export default function ProjectDeployments({ project, selectedId, onSelect, openService, addService, notify }: { project:Project;selectedId:string|null;onSelect:(id:string)=>void;openService:(service:ServiceProfile)=>void;addService:()=>void;notify:(message:string)=>void }) {
  const services = project.services || [];
  const active = services.find((service) => service.id === selectedId) || services[0];
  return <div className="deploy-workspace" aria-label="Project deployment workspace"><section className="panel deploy-service-selector"><div className="deploy-service-picker"><label>Deploy service<select aria-label="Deploy service" value={active?.id || ""} disabled={!services.length} onChange={(event) => onSelect(event.target.value)}>{!services.length && <option value="">No services added</option>}{groups.map((group) => {const members = services.filter((service) => serviceScope(service.role) === group.scope);return members.length ? <optgroup label={group.label} key={group.scope}>{members.map((service) => <option value={service.id} key={service.id}>{service.name}</option>)}</optgroup> : null;})}</select></label>{active && <div><strong>{active.name}</strong><small>{groups.find((group) => group.scope === serviceScope(active.role))?.label} · {active.runtime || "Runtime not set"}</small></div>}</div>{active ? <button className="button integration-secondary" onClick={() => openService(active)}>Open local service<Icon name="arrow" size={14}/></button> : <button className="button integration-action" onClick={addService}><Icon name="plus" size={14}/>Add frontend service</button>}</section>{active && <ServiceDeployWorkspace key={active.id} projectId={project.id} profile={active} notify={notify}/>}</div>;
}

function ServiceDeployWorkspace({ projectId, profile, notify }: { projectId:string;profile:ServiceProfile;notify:(message:string)=>void }) {
  const storage = useWorkspaceStorage();
  const [view,setView] = useState<"Setup"|"Release"|"Activity">("Setup");
  const [route,setRoute] = useState<ReleaseRoute>(profile.release?.route || "github");
  const [selectedRelease,setSelectedRelease] = useState("");
  const [notice,setNotice] = useState("");
  useEffect(() => {
    const pushed = (event:Event) => {const detail = (event as CustomEvent<{projectId:string;serviceId:string}>).detail;if(detail?.projectId === projectId && detail.serviceId === profile.id){setSelectedRelease("");setNotice("GitHub push accepted. Checking for a matching deployment.");setView("Activity");}};
    window.addEventListener("gitme:git-pushed",pushed);return () => window.removeEventListener("gitme:git-pushed",pushed);
  },[projectId,profile.id]);
  function saveFlow() {
    if (storage.state !== "saved") return;
    const saved = updateWorkspace((data) => ({...data,projects:data.projects.map((project) => project.id !== projectId ? project : {...project,services:project.services?.map((service) => service.id !== profile.id ? service : {...service,release:{route,target:service.release?.target || "preview",branch:service.release?.branch || service.branch}})})}));
    notify(saved ? "Default release flow saved." : "Flow changed for this session only. Browser storage is unavailable.");
  }
  function requested(result:DeploymentAction) {setNotice(result.warning || result.result.message);setView("Activity");}
  const trackerProps = {projectId,profile,selectedRelease,onSelectRelease:setSelectedRelease,onSetup:()=>setView("Setup"),onActivity:()=>setView("Activity"),onRequested:requested};
  return <div className="service-deploy-workspace"><div className="deploy-area-tabs" role="tablist" aria-label="Deployment areas" onKeyDown={(event) => {const areas = ["Setup","Release","Activity"] as const;const index = areas.indexOf(view);const next = event.key === "ArrowRight" ? (index+1)%3 : event.key === "ArrowLeft" ? (index+2)%3 : event.key === "Home" ? 0 : event.key === "End" ? 2 : -1;if(next >= 0){event.preventDefault();setView(areas[next]);document.getElementById(`deploy-tab-${areas[next]}`)?.focus();}}}>{(["Setup","Release","Activity"] as const).map((area) => <button type="button" key={area} role="tab" id={`deploy-tab-${area}`} aria-selected={view === area} tabIndex={view === area ? 0 : -1} aria-controls={`deploy-area-${area}`} className={view === area ? "selected" : ""} onClick={() => setView(area)}>{area}</button>)}</div><div role="tabpanel" id={`deploy-area-${view}`} aria-labelledby={`deploy-tab-${view}`}>
    {view === "Setup" && <><GitHubRepository projectId={projectId} serviceId={profile.id} mode="connection"/><VercelDeployment projectId={projectId} serviceId={profile.id}/><DeploymentSettings projectId={projectId} profile={profile} notify={notify}/></>}
    {view === "Release" && <><section className="panel deploy-flow-picker" aria-label="Deployment flow"><div className="panel-heading"><h2>Release flow</h2><button className="button integration-action" disabled={storage.state !== "saved" || route === profile.release?.route} onClick={saveFlow}><Icon name="check" size={14}/>Save default flow</button></div><div className="deploy-flow-body"><div className="release-routes" role="group" aria-label="Deployment flow choice">{flows.map((flow) => <button type="button" key={flow.id} className={`release-route ${route === flow.id ? "selected" : ""}`} aria-pressed={route === flow.id} onClick={() => {setRoute(flow.id);setNotice("");}}><span className="release-route-title"><Icon name={flow.icon} size={16}/>{flow.title}</span><span>{flow.description}</span>{profile.release?.route === flow.id && <small>Default</small>}</button>)}</div>{route === "github" && <><p className="git-help">Your host&apos;s branch rules choose Preview or Production. Saving this flow does not change those rules.</p>{!profile.github && <button className="text-link" onClick={() => setView("Setup")}><Icon name="link" size={13}/>Connect GitHub in Setup</button>}{!profile.vercel && <button className="text-link" onClick={() => setView("Setup")}><Icon name="link" size={13}/>Connect deployment in Setup</button>}</>}{route === "vercel-remote" && <p className="git-help">Choose a recorded GitHub commit below, then review its Preview or Production target.</p>}{route === "vercel-local" && <p className="git-help">Review current files and choose Preview or Production below.</p>}</div></section>{route === "github" && <LocalGit projectId={projectId} profile={profile}/>} {route === "vercel-remote" && <DeploymentTracker {...trackerProps} view="release"/>}{route === "vercel-local" && <LocalUpload projectId={projectId} profile={profile} onSetup={() => setView("Setup")} onActivity={() => setView("Activity")} onRequested={requested}/>}</>}
    {view === "Activity" && <>{notice && <p className="git-notice" role="status">{notice}</p>}<VercelDeployment projectId={projectId} serviceId={profile.id} mode="status" onSetup={() => setView("Setup")}/><DeploymentTracker {...trackerProps} view="activity"/><DeploymentRequestHistory projectId={projectId} serviceId={profile.id}/></>}
  </div>{(["Setup","Release","Activity"] as const).filter((area) => area !== view).map((area) => <div key={area} role="tabpanel" id={`deploy-area-${area}`} aria-labelledby={`deploy-tab-${area}`} hidden/>)}</div>;
}



