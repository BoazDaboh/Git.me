"use client";

import { useEffect, useRef, useState } from "react";
import { trackingRequest } from "@/lib/deployment-tracking";
import type { DeploymentAction, LocalUploadPlan } from "@/lib/manual-deployment";
import { useWorkspaceStorage } from "@/lib/store";
import type { ServiceProfile } from "@/lib/services";
import Icon from "./Icon";

export function uploadSize(bytes:number) { return bytes < 1024 ? `${bytes} B` : bytes < 1024*1024 ? `${(bytes/1024).toFixed(1)} KiB` : `${(bytes/(1024*1024)).toFixed(1)} MiB`; }

export default function LocalUpload({projectId,profile,onSetup,onRequested,onActivity}:{projectId:string;profile:ServiceProfile;onSetup:()=>void;onRequested:(result:DeploymentAction)=>void;onActivity:()=>void}) {
  const storage = useWorkspaceStorage();
  const [target,setTarget] = useState<"preview"|"production">("preview");
  const [plan,setPlan] = useState<LocalUploadPlan|null>(null);
  const [phase,setPhase] = useState<"idle"|"review"|"upload">("idle");
  const [error,setError] = useState("");
  const busy = useRef(false);
  const request = useRef<AbortController|null>(null);
  const livePlan = useRef<LocalUploadPlan|null>(null);
  const disabled = storage.state !== "saved" || !profile.vercel || !profile.folder;
  const binding = JSON.stringify([profile.folder,profile.vercel]);
  useEffect(() => () => {
    request.current?.abort();
    if (livePlan.current) void trackingRequest("local-cancel",{project_id:projectId,service_id:profile.id,token:livePlan.current.token}).catch(()=>{});
  },[projectId,profile.id]);
  async function review() {
    if (disabled || busy.current) return;
    busy.current = true;setPhase("review");setError("");
    const controller = new AbortController();request.current = controller;
    try {const next = await trackingRequest<LocalUploadPlan>("local-review",{project_id:projectId,service_id:profile.id,environment:target},controller.signal);if (!controller.signal.aborted){livePlan.current=next;setPlan(next);}}
    catch(error){if (!controller.signal.aborted)setError(error instanceof Error ? error.message : "File review unavailable.");}
    finally{busy.current=false;if(!controller.signal.aborted)setPhase("idle");}
  }
  function close() {
    if (!plan || busy.current) return;
    void trackingRequest("local-cancel",{project_id:projectId,service_id:profile.id,token:plan.token}).catch(()=>{});livePlan.current=null;setPlan(null);
  }
  async function upload() {
    if (!plan || disabled || busy.current) return;
    busy.current=true;setPhase("upload");setError("");
    const controller=new AbortController();request.current=controller;
    try {
      const result=await trackingRequest<DeploymentAction>("local-execute",{project_id:projectId,service_id:profile.id,token:plan.token},controller.signal);
      if(controller.signal.aborted)return;
      livePlan.current=null;setPlan(null);onRequested(result);
      window.dispatchEvent(new CustomEvent("gitme:deployment-requested",{detail:{projectId,serviceId:profile.id}}));
    } catch(error){if(!controller.signal.aborted){livePlan.current=null;setPlan(null);setError(`${error instanceof Error ? error.message : "Upload confirmation unavailable."} Check Activity and Vercel before retrying.`);}}
    finally{busy.current=false;if(!controller.signal.aborted)setPhase("idle");}
  }
  return <section className="panel local-upload" aria-label="Local upload"><div className="panel-heading"><h2>Local source</h2><span className="badge">Vercel</span></div><div className="deploy-settings-body"><p className="git-help">Upload this service&apos;s current files, including uncommitted changes. GitHub and local Git history stay unchanged.</p><p className="git-repository-path"><code>{profile.folder || "Set a service folder first"}</code></p><p className="git-help">Environment files, credentials, dependencies and build output are excluded. Ignore files add exclusions; ! rules cannot reinclude excluded files. Review the full file list before uploading.</p><div className="manual-deploy-actions"><label>Target<select aria-label="Local upload target" value={target} disabled={phase!=="idle"} onChange={event=>setTarget(event.target.value as "preview"|"production")}><option value="preview">Preview</option><option value="production">Production</option></select></label><button className="button primary integration-action" disabled={disabled || phase!=="idle"} onClick={()=>void review()}><Icon name="upload" size={15}/>{phase==="review" ? "Reviewing files..." : "Review local files"}</button></div>{!profile.vercel && <button className="text-link" onClick={onSetup}><Icon name="link" size={13}/>Connect deployment in Setup</button>}<p className="git-help">Up to 2,000 files, 32 MiB total and 8 MiB per file. Nested Vercel Root Directories use the GitHub flow for now.</p>{error && <p className="github-error" role="alert">{error}</p>}<button className="text-link" onClick={onActivity}>Open activity<Icon name="arrow" size={13}/></button></div>{plan && <FileReview key={plan.token} plan={plan} binding={binding} busy={phase==="upload"} disabled={disabled} close={close} upload={()=>void upload()}/>}</section>;
}

function FileReview({plan,binding,busy,disabled,close,upload}:{plan:LocalUploadPlan;binding:string;busy:boolean;disabled:boolean;close:()=>void;upload:()=>void}) {
  const dialog=useRef<HTMLDialogElement>(null);
  const [original]=useState(binding);
  const [expired,setExpired]=useState(false);
  const [acknowledged,setAcknowledged]=useState(false);
  const changed=original!==binding;
  useEffect(()=>{const node=dialog.current;node?.showModal();const timer=setTimeout(()=>setExpired(true),Math.max(0,new Date(plan.expires_at).getTime()-Date.now()));return()=>{clearTimeout(timer);node?.close();};},[plan]);
  return <dialog ref={dialog} className="dialog git-review local-upload-review" aria-labelledby="local-upload-title" onCancel={event=>{event.preventDefault();if(!busy)close();}}><div className="dialog-heading"><h2 id="local-upload-title">Review local upload</h2><button className="icon-button" aria-label="Close file review" disabled={busy} onClick={close}><Icon name="close"/></button></div><div className="git-review-body"><dl><dt>Project</dt><dd>{plan.link.scopeSlug}/{plan.link.projectName}</dd><dt>Account</dt><dd>{plan.link.connectionId}</dd><dt>Target</dt><dd>{plan.target === "production" ? "Production" : "Preview"}</dd><dt>Folder</dt><dd><code>{plan.source.folder}</code></dd><dt>Source</dt><dd>{plan.source.files.length} files · {uploadSize(plan.source.bytes)}</dd></dl><p className="git-notice">{plan.target === "production" ? "A successful Production build may replace the version on your live domain." : "This requests a Preview deployment without changing the production domain."}</p><details open><summary>Files to upload ({plan.source.files.length})</summary><ul className="upload-file-list">{plan.source.files.map(file=><li key={file.path}><code>{file.path}</code><small>{uploadSize(file.size)}</small></li>)}</ul></details><details><summary>Excluded ({plan.source.excluded_count})</summary><ul className="upload-file-list">{plan.source.excluded.map(file=><li key={file.path}><code>{file.path}</code><small>{file.reason}</small></li>)}</ul>{plan.source.excluded_count>plan.source.excluded.length && <p className="git-help">Showing the first {plan.source.excluded.length} exclusions.</p>}</details><p className="git-help">Files upload only after confirmation. Vercel builds with its saved settings and environment variables. Some source files may remain stored on Vercel if an upload stops. Git/Me does not retry automatically.</p><label className="upload-acknowledge"><input type="checkbox" checked={acknowledged} disabled={busy} onChange={event=>setAcknowledged(event.target.checked)}/>I reviewed these files and their destination.</label>{(expired||changed) && <p className="github-error" role="alert">{expired ? "Review expired." : "Service settings changed."} Close and review again.</p>}{busy && <p className="git-feedback" role="status">Uploading source and requesting the build...</p>}</div><div className="dialog-footer"><button className="button integration-action" disabled={busy} onClick={close}>Cancel</button><button className="button primary integration-action" disabled={busy||disabled||expired||changed||!acknowledged} onClick={upload}><Icon name="upload" size={15}/>{busy ? "Uploading..." : `Confirm ${plan.target} upload`}</button></div></dialog>;
}
