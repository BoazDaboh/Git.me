"use client";
import { useEffect, useRef, useState } from "react";
import { useWorkspaceStorage } from "@/lib/store";
import { processRequest, type EndpointHealth } from "@/lib/processes";
import { vercelRequest, type VercelActivity } from "@/lib/vercel";
import type { Project } from "@/lib/workspace";

type State = "unknown" | "ok" | "problem";
type Reading = { state: State; text: string };
const unknown = (text: string): Reading => ({ state: "unknown", text });
export default function ProjectStatusDots({ project }: { project: Project }) {
  const storage = useWorkspaceStorage();
  const dialog = useRef<HTMLDialogElement>(null);
  const [refreshCount, setRefreshCount] = useState(0);
  const [checkedAt, setCheckedAt] = useState<number | null>(null);
  const [deployments, setDeployments] = useState<{id:string;name:string;provider:string;reading:Reading}[]>([]);
  const [readings, setReadings] = useState<Reading[]>([unknown("Web: not checked"), unknown("API: not checked"), unknown("Deployment: not checked")]);
  const services = JSON.stringify((project.services ?? []).map(s => ({ id:s.id, name:s.name, role:s.role, port:s.port, url:s.environments.local.url, health:s.healthPath, vercel:s.vercel, provider:s.deployment.provider, productionUrl:s.environments.production.url })));
  useEffect(() => {
    if (storage.state !== "saved") return;
    const profiles: {id:string;name:string;role:string;port:number|null;url:string;health:string;vercel?:unknown;provider:string;productionUrl:string}[] = JSON.parse(services);
    const controller = new AbortController(); let active = true;
    const endpoint = (profile: typeof profiles[number]) => {
      const raw = profile.url || (profile.port ? `http://127.0.0.1:${profile.port}` : "");
      try { const url = new URL(raw); return ["http:","https:"].includes(url.protocol) && !url.username && !url.password && ["localhost","127.0.0.1","[::1]","::1"].includes(url.hostname.toLowerCase()); } catch { return false; }
    };
    const check = async () => {
      const checks = async (role:string, label:string):Promise<Reading> => {
        const selected = profiles.filter(p => p.role === role && endpoint(p));
        if (!selected.length) return unknown(`${label}: no local endpoint configured`);
        if (selected.length > 8) return unknown(`${label}: too many services to summarize`);
        try { const results = await Promise.all(selected.map(p => processRequest<EndpointHealth>("/services/health", {project_id:project.id,service_id:p.id}, controller.signal))); const good = results.every(r => r.responding); return {state:good?"ok":"problem",text:`${label}: ${good?"all local endpoints responding":"a local endpoint is not healthy"}`}; } catch { return unknown(`${label}: status unavailable`); }
      };
      const deployment = async ():Promise<Reading> => {
        const targets = profiles.filter(p => p.vercel || p.provider || p.productionUrl);
        if (!targets.length) { if(active) setDeployments([]); return unknown("Deployment: no services configured"); }
        if (targets.length > 8) { if(active) setDeployments([]); return unknown("Deployment: too many services to summarize"); }
        const details = await Promise.all(targets.map(async p => {
          const provider = p.vercel ? "Vercel" : (p.provider || "Provider not set");
          let reading:Reading = unknown(p.vercel ? "Not checked" : "Automatic check unavailable");
          if (p.vercel) try {
            const result = await vercelRequest<VercelActivity>(`activity?${new URLSearchParams({project_id:project.id,service_id:p.id, ...(refreshCount ? {refresh:"1"} : {})})}`, undefined, controller.signal);
            const state = result.production?.state.toUpperCase();
            reading = state === "READY" ? {state:"ok",text:"Production ready"} : state && ["ERROR","CANCELED"].includes(state) ? {state:"problem",text:`Production ${state.toLowerCase()}`} : unknown(state ? `Production ${state.toLowerCase()}` : "No production deployment reported");
          } catch { reading = unknown("Connection unavailable or not verified"); }
          return {id:p.id,name:p.name,provider,reading};
        }));
        if (active) setDeployments(details);
        if (details.some(d => d.reading.state === "problem")) return {state:"problem",text:"Deployment: a service reported a failure"};
        if (details.some(d => d.reading.state === "unknown")) return unknown("Deployment: some services are not verified");
        return {state:"ok",text:"Deployment: all configured services report ready"};
      };
      const next = await Promise.all([checks("frontend","Web"),checks("backend","API"),deployment()]); if(active) { setReadings(next); setCheckedAt(Date.now()); }
    };
    void check(); const timer = setInterval(() => { if (document.visibilityState === "visible") void check(); }, 30*60*1000);
    return () => { active=false;controller.abort();clearInterval(timer); };
  },[project.id,services,storage.state,refreshCount]);
  const shown = storage.state === "saved" ? readings : [unknown("Web: waiting for workspace save"), unknown("API: waiting for workspace save"), unknown("Deployment: waiting for workspace save")];
  return <div className="project-status-dots"><button type="button" className="project-status-trigger" aria-label={`View health for ${project.name}`} onClick={() => dialog.current?.showModal()}>{shown.map((reading,index) => <span key={index} className={`project-status-dot ${reading.state}`} />)}</button>
    <dialog ref={dialog} className="dialog project-health-dialog" aria-label={`${project.name} health`} onClick={(event) => { if (event.target === dialog.current) dialog.current?.close(); }}><div className="dialog-heading"><h2>Project health</h2><button type="button" className="icon-button" aria-label="Close" onClick={() => dialog.current?.close()}>×</button></div><div className="project-health-content">{shown.map((reading,index) => <div className="project-health-row" key={index}><span className={`project-status-dot ${reading.state}`}/><strong>{["Local frontend", "Local API", "Deployment"][index]}</strong><span>{reading.text.split(": ").slice(1).join(": ")}</span></div>)}{deployments.length > 0 && <section className="project-health-deployments"><strong>Deployment services</strong>{deployments.map(item => <div className="project-health-row" key={item.id}><span className={`project-status-dot ${item.reading.state}`}/><strong>{item.name}</strong><span>{item.provider}: {item.reading.text}</span></div>)}</section>}<p>{checkedAt && storage.state === "saved" ? `Checked ${new Date(checkedAt).toLocaleTimeString()}` : "Not checked yet"}</p><button type="button" className="button" disabled={storage.state !== "saved"} onClick={() => setRefreshCount(value => value + 1)}>Refresh</button><details><summary>Health setup guide</summary><p>In your project, open the frontend or backend service and set its local URL or port. Start the server, then refresh this check. For an API, set a health path such as <code>/health</code>.</p><p>A health endpoint can return HTTP 200 and a short response such as <code>{'{"status":"ok"}'}</code>. Your backend must implement that route. A frontend can use its existing home page if it returns a successful response.</p><p>Add a deployment provider to each service under Deploy. Automatic production status is currently supported for linked Vercel projects; other providers remain unverified until their integrations are added. A ready deployment does not guarantee every feature works.</p></details></div></dialog></div>;
}
