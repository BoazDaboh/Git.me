"use client";

import { useEffect, useRef, useState } from "react";
import { trackingRequest, type ReleaseTracking } from "@/lib/deployment-tracking";
import { deploymentURL, projectURL } from "@/lib/vercel";
import type { ServiceProfile } from "@/lib/services";
import { useWorkspaceStorage } from "@/lib/store";
import { useVercel } from "./VercelConnection";
import Icon from "./Icon";
import ManualDeployment from "./ManualDeployment";
import type { DeploymentAction } from "@/lib/manual-deployment";

type TrackerProps = { projectId:string;profile:ServiceProfile;view:"activity"|"release";selectedRelease:string;onSelectRelease:(id:string)=>void;onSetup:()=>void;onActivity:()=>void;onRequested:(result:DeploymentAction)=>void };
export default function DeploymentTracker(props:TrackerProps) {
  const { projectId,profile } = props;
  return <Tracker key={JSON.stringify([projectId, profile.id, profile.repository, profile.github, profile.vercel])} {...props}/>;
}
function Tracker({ projectId, profile, view, selectedRelease:selected, onSelectRelease:setSelected, onSetup, onActivity, onRequested }:TrackerProps) {
  const storage = useWorkspaceStorage(); const cloud = useVercel();
  const account = cloud.connections.find((item) => item.id === profile.vercel?.connectionId);
  const accountIdentity = `${cloud.available}:${account?.id}:${account?.connected}:${account?.checked_at}`;
  const canRead = cloud.available && !!account?.connected;
  const [data,setData] = useState<ReleaseTracking | null>(null);
  const [loadedSelection,setLoadedSelection] = useState<string | null>(null);
  const [loading,setLoading] = useState(false);
  const [tracking,setTracking] = useState(false);
  const [error,setError] = useState("");
  const [revision,setRevision] = useState(0);
  const refresh = useRef<() => Promise<void>>(async () => {});
  const trackingRequestRef = useRef<AbortController | null>(null);
  useEffect(() => () => trackingRequestRef.current?.abort(), []);
  useEffect(() => {
    if (storage.state !== "saved") return;
    let active = true, busy = false, pending = false, due = 0; let controller: AbortController | null = null;
    const check = async () => {
      if (!active) return;
      if (busy) { pending = true; return; }
      busy = true; due = Date.now()+15000; controller = new AbortController(); setLoading(true);
      try {
        const params = new URLSearchParams({ project_id: projectId, service_id: profile.id, ...(selected ? { release_id: selected } : {}) });
        const result = await trackingRequest<ReleaseTracking>(`tracking?${params}`, undefined, controller.signal);
        if (!active) return;
        setData(result);setLoadedSelection(selected); setError("");
        const deployments = result.release?.reading?.deployments || [];
        const settled = deployments.length > 0 && deployments.every((item) => ["READY","ERROR","CANCELED","BLOCKED"].includes(item.state));
        due = Date.now() + (settled || !result.release || ["not-observed","unlinked","link-changed","source-changed"].includes(result.release.state) ? 60000 : 15000);
      } catch (error) { if (active) { setError(error instanceof Error ? error.message : "Tracking unavailable."); due = Date.now()+30000; } }
      finally { busy = false; if (active) { setLoading(false); if (pending) { pending = false; void check(); } } }
    };
    refresh.current = check;
    const wake = () => { if (document.visibilityState === "visible") void check(); };
    const pushed = (event: Event) => { const detail = (event as CustomEvent<{ projectId: string; serviceId: string }>).detail; if (detail?.projectId === projectId && detail.serviceId === profile.id) { setSelected(""); if (!selected) void check(); } };
    const first = setTimeout(() => { void check(); },0);
    const timer = setInterval(() => { if (document.visibilityState === "visible" && Date.now() >= due) void check(); },5000);
    window.addEventListener("focus",wake);window.addEventListener("online",wake);window.addEventListener("gitme:companion-ready",wake);document.addEventListener("visibilitychange",wake);window.addEventListener("gitme:git-pushed",pushed);
    return () => { active = false;controller?.abort();clearTimeout(first);clearInterval(timer);window.removeEventListener("focus",wake);window.removeEventListener("online",wake);window.removeEventListener("gitme:companion-ready",wake);document.removeEventListener("visibilitychange",wake);window.removeEventListener("gitme:git-pushed",pushed); };
  },[projectId,profile.id,selected,setSelected,storage.state,accountIdentity,revision]);
  async function trackCurrent() {
    const controller = new AbortController(); trackingRequestRef.current = controller; setTracking(true);setError("");
    try { await trackingRequest("track-current",{project_id:projectId,service_id:profile.id},controller.signal); if (!controller.signal.aborted) { setSelected("");setRevision((value) => value+1); } }
    catch(error) { if (!controller.signal.aborted) setError(error instanceof Error ? error.message : "Could not track the current commit."); }
    finally { if (!controller.signal.aborted) setTracking(false); }
  }
  const selectionCurrent = loadedSelection === selected;
  const release = selectionCurrent ? data?.release : undefined;
  const reading = !error && canRead ? release?.reading : null;
  if (view === "release") return <section className="panel deployment-tracker" aria-label="Remote deployment source"><div className="panel-heading"><h2>Choose GitHub commit</h2><div className="github-actions"><button className="button integration-action" disabled={storage.state !== "saved" || loading || tracking} onClick={() => {void refresh.current();}}><Icon name="refresh" size={14}/>{loading ? "Checking..." : "Refresh commits"}</button><button className="button integration-secondary" disabled={storage.state !== "saved" || tracking || !profile.folder.trim()} onClick={() => {void trackCurrent();}}><Icon name="branch" size={14}/>{tracking ? "Verifying GitHub..." : "Track current GitHub commit"}</button></div></div><div className="deployment-tracker-body">{data && data.history.length > 0 && <label className="release-history-picker">Recorded commit<select aria-label="Tracked release" value={selected} onChange={(event) => setSelected(event.target.value)}><option value="">Latest recorded release</option>{data.history.map((item) => <option key={item.id} value={item.id}>{item.branch} · {item.commit.slice(0,7)} · {new Date(item.created_at).toLocaleString("en-GB")}</option>)}</select></label>}{loading && !data && <p className="git-help" role="status">Loading recorded commits...</p>}{release && <p className="git-help">{release.message}</p>}{!profile.vercel && <button className="button integration-secondary" onClick={onSetup}><Icon name="link" size={14}/>Connect deployment in Setup</button>}<ManualDeployment key={release?.id || "no-release"} projectId={projectId} serviceId={profile.id} release={release || null} disabled={storage.state !== "saved" || !canRead || loading || !selectionCurrent || !!error} onRequested={(result) => {setRevision((value) => value+1);onRequested(result);}} onActivity={onActivity}/>{error && <p className="github-error" role="alert">{error}</p>}</div></section>;
  return <section className="panel deployment-tracker" aria-label="Deployment tracking"><div className="panel-heading"><h2><Icon name="globe" size={16}/>Deployment tracking</h2><div className="github-actions"><button className="button integration-action" disabled={storage.state !== "saved" || loading || tracking} onClick={() => { void refresh.current(); }}><Icon name="refresh" size={15}/>{loading ? "Checking..." : "Refresh"}</button><button className="button integration-secondary" disabled={storage.state !== "saved" || tracking || !profile.folder.trim()} onClick={() => { void trackCurrent(); }}><Icon name="branch" size={15}/>{tracking ? "Verifying GitHub..." : "Track current GitHub commit"}</button></div></div>
    <div className="deployment-tracker-body">
      {data && data.history.length > 1 && <label className="release-history-picker">Release<select aria-label="Tracked release" value={selected} onChange={(event) => setSelected(event.target.value)}><option value="">Latest recorded release</option>{data.history.map((item) => <option key={item.id} value={item.id}>{item.branch} · {item.commit.slice(0,7)} · {new Date(item.created_at).toLocaleString("en-GB")}</option>)}</select></label>}
      {!data && loading && <p className="git-help" role="status">Loading tracked releases...</p>}
      {data && !release && <p className="git-help">Push through Git/Me to start tracking, or verify a commit already on GitHub.</p>}
      {release && <><div className="tracked-release-source"><span><Icon name="branch" size={14}/>{release.branch}</span><code>{release.commit.slice(0,7)}</code><a className="text-link" href={`${release.repository.replace(/\.git$/,"")}/commit/${release.commit}`} target="_blank" rel="noreferrer">View commit<Icon name="external" size={13}/></a></div>
        <div className="tracked-push" data-verified={release.remote_verified}><Icon name={release.remote_verified ? "check" : "clock"} size={16}/><div><strong>{release.source === "push" ? "GitHub push accepted" : "Existing GitHub commit verified"}</strong><small>{new Date(release.created_at).toLocaleString("en-GB")}{release.source === "push" && !release.remote_verified && " · Follow-up branch verification was unavailable"}</small></div></div>
        <p className="git-help" role="status">{release.message}</p>
        {release.target && <div className="tracked-target"><span>Vercel · {release.target.scopeSlug}/{release.target.projectName}</span><a className="text-link" href={projectURL(release.target.scopeSlug,release.target.projectName)} target="_blank" rel="noreferrer">Open Vercel<Icon name="external" size={13}/></a></div>}
        {reading?.deployments.map((deployment) => <div className="tracked-deployment" key={deployment.id}><div><span className="tracked-deployment-state" data-state={deployment.state}>{({READY:"Ready",BUILDING:"Building",QUEUED:"Queued",INITIALIZING:"Initializing",ERROR:"Build failed",CANCELED:"Canceled",BLOCKED:"Blocked",UNKNOWN:"Unknown"} as Record<string,string>)[deployment.state] || "Unknown"}</span><span className="vercel-target">{deployment.target}</span>{deployment.state === "READY" && deployment.id === reading.production_id && deployment.target === "production" && <span className="tracked-live">Serving production</span>}</div><code>{deployment.id}</code><small>{new Date(deployment.created_at).toLocaleString("en-GB")}</small>{deploymentURL(deployment.url) && <a className="button integration-secondary" href={deploymentURL(deployment.url)} target="_blank" rel="noreferrer"><Icon name="external" size={14}/>Open deployment</a>}</div>)}
        {reading && <p className="git-help">Last checked {new Date(reading.checked_at).toLocaleTimeString("en-GB")}</p>}
        {reading && reading.warnings.length > 0 && <ul className="github-warnings">{reading.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>}
        {release.target && !canRead && <p className="git-help">The linked Vercel account is unavailable. Reconnect it in Settings to read deployment status.</p>}
      </>}
      {(!profile.vercel || release?.state === "link-changed") && <button className="button integration-secondary" onClick={onSetup}><Icon name="link" size={15}/>{profile.vercel ? "Review deployment link in Setup" : "Connect deployment in Setup"}</button>}
      {error && <p className="github-error" role="alert">{error}</p>}
    </div>
  </section>;
}
