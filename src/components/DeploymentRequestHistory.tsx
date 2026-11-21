"use client";

import { useEffect, useRef, useState } from "react";
import { trackingRequest } from "@/lib/deployment-tracking";
import type { DeploymentAction } from "@/lib/manual-deployment";
import { useWorkspaceStorage } from "@/lib/store";
import { deploymentURL, projectURL } from "@/lib/vercel";
import Icon from "./Icon";
import { uploadSize } from "./LocalUpload";

export default function DeploymentRequestHistory({ projectId, serviceId }: { projectId:string;serviceId:string }) {
  const storage = useWorkspaceStorage();
  const [actions,setActions] = useState<DeploymentAction[] | null>(null);
  const [error,setError] = useState("");
  const [loading,setLoading] = useState(false);
  const refresh = useRef<() => Promise<void>>(async () => {});
  useEffect(() => {
    if (storage.state !== "saved") return;
    let active = true,busy = false;let controller:AbortController | null = null;
    const check = async () => {
      if (!active || busy) return;busy = true;controller = new AbortController();setLoading(true);
      try {const data = await trackingRequest<{actions:DeploymentAction[]}>(`deploy-actions?${new URLSearchParams({project_id:projectId,service_id:serviceId})}`,undefined,controller.signal);if(active){setActions(data.actions);setError("");}}
      catch(error){if(active)setError(error instanceof Error ? error.message : "Deployment request history unavailable.");}
      finally{busy = false;if(active)setLoading(false);}
    };
    refresh.current = check;
    const wake = () => {if(document.visibilityState === "visible")void check();};
    const first = setTimeout(wake,0),timer = setInterval(wake,60000);
    window.addEventListener("focus",wake);window.addEventListener("online",wake);window.addEventListener("gitme:companion-ready",wake);
    return () => {active = false;controller?.abort();clearTimeout(first);clearInterval(timer);window.removeEventListener("focus",wake);window.removeEventListener("online",wake);window.removeEventListener("gitme:companion-ready",wake);refresh.current = async () => {};};
  },[projectId,serviceId,storage.state]);
  return <section className="panel deployment-requests" aria-label="Deployment request history"><div className="panel-heading"><h2>Deployment requests</h2><button className="button integration-action" disabled={loading || storage.state !== "saved"} onClick={() => {void refresh.current();}}><Icon name="refresh" size={14}/>{loading ? "Refreshing..." : "Refresh requests"}</button></div><div className="deployment-requests-body">{actions?.map((action) => <RequestRecord key={action.id} action={action} projectId={projectId} serviceId={serviceId} refreshVersion={actions}/>)}{actions?.length === 0 && <p className="git-help">No manual deployment requests for this service.</p>}{!actions && loading && <p className="git-help" role="status">Loading request history...</p>}{error && <p className="github-error" role="alert">{error}</p>}</div></section>;
}

function RequestRecord({action,projectId,serviceId,refreshVersion}:{action:DeploymentAction;projectId:string;serviceId:string;refreshVersion:DeploymentAction[]}) {
  const [reading,setReading] = useState<{state:string;url:string;production:boolean}|null>(null);
  const [error,setError] = useState("");
  const localAccepted = !!action.local && action.result.outcome === "accepted" && !!action.result.deployment_id;
  useEffect(()=>{
    if(!localAccepted)return;
    let active=true;const controller=new AbortController();
    const check=async()=>{try{const next=await trackingRequest<{state:string;url:string;production:boolean}>(`local-status?${new URLSearchParams({project_id:projectId,service_id:serviceId,action_id:action.id})}`,undefined,controller.signal);if(active){setReading(next);setError("");}}catch(error){if(active){setReading(null);setError(error instanceof Error?error.message:"Build status unavailable.");}}};
    const timer=setTimeout(()=>{if(document.visibilityState==="visible")void check();},0);
    return()=>{active=false;clearTimeout(timer);controller.abort();};
  },[localAccepted,projectId,serviceId,action.id,refreshVersion]);
  const target=action.local?.target || action.release.target;
  const url=deploymentURL(reading?.url || action.result.url);
  return <article className="deployment-action-record"><div><strong>{({accepted:"Request accepted",rejected:"Request rejected",unconfirmed:"Confirmation unavailable",submitting:"Submission unconfirmed","not-sent":"Not submitted"})[action.result.outcome]}</strong><span className="badge">{action.environment}</span>{action.local && <span className="badge">Local upload</span>}</div><small>{action.local ? `${action.local.files} files · ${uploadSize(action.local.bytes)} · ${action.local.digest.slice(0,12)}` : `${action.release.branch} · ${action.release.commit.slice(0,7)}`} · {new Date(action.created_at).toLocaleString("en-GB")}</small>{action.result.deployment_id && <code>{action.result.deployment_id}</code>}{reading && <p className="vercel-state" data-state={reading.state}>{reading.state}{reading.production ? " · Current production" : ""}</p>}{localAccepted && !reading && !error && <p className="git-help" role="status">Checking build...</p>}<p className="git-help">{action.result.message}</p>{error && <p className="github-error" role="alert">{error}</p>}<div className="github-actions">{url && <a className="button integration-secondary" href={url} target="_blank" rel="noreferrer">Open deployment<Icon name="external" size={13}/></a>}{target && <a className="text-link" href={projectURL(target.scopeSlug,target.projectName)} target="_blank" rel="noreferrer">Open Vercel<Icon name="external" size={13}/></a>}</div></article>;
}
