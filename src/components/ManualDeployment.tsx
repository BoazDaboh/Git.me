"use client";

import { useEffect, useRef, useState } from "react";
import { trackingRequest, type TrackedRelease } from "@/lib/deployment-tracking";
import type { DeploymentAction, DeploymentPlan } from "@/lib/manual-deployment";
import Icon from "./Icon";

export default function ManualDeployment({ projectId, serviceId, release, disabled, onRequested, onActivity }: { projectId: string; serviceId: string; release: TrackedRelease | null; disabled: boolean; onRequested: (result:DeploymentAction) => void; onActivity: () => void }) {
  const [target,setTarget] = useState<"preview"|"production">("preview");
  const [plan,setPlan] = useState<DeploymentPlan | null>(null);
  const [busy,setBusy] = useState(false);
  const [error,setError] = useState("");
  const [notice,setNotice] = useState("");
  const submitting = useRef(false);
  const request = useRef<AbortController | null>(null);
  const livePlan = useRef<DeploymentPlan | null>(null);
  const selection = {project_id:projectId,service_id:serviceId};
  useEffect(() => {livePlan.current = plan;},[plan]);
  useEffect(() => () => {
    request.current?.abort();
    if (livePlan.current?.token) void trackingRequest("deploy-cancel",{project_id:projectId,service_id:serviceId,token:livePlan.current.token}).catch(() => {});
  },[projectId,serviceId]);
  async function review() {
    if (!release || disabled || submitting.current) return;
    submitting.current = true;setBusy(true);setError("");setNotice("");
    const controller = new AbortController();request.current = controller;
    try {const next = await trackingRequest<DeploymentPlan>("deploy-review",{...selection,release_id:release.id,environment:target},controller.signal);if (!controller.signal.aborted) setPlan(next);}
    catch(error) {if (!controller.signal.aborted) setError(error instanceof Error ? error.message : "Deployment review unavailable.");}
    finally {submitting.current = false;if (!controller.signal.aborted) setBusy(false);}
  }
  function close() {
    if (!plan || submitting.current) return;
    void trackingRequest("deploy-cancel",{...selection,token:plan.token}).catch(() => {});setPlan(null);
  }
  async function deploy() {
    if (!plan || disabled || submitting.current) return;
    submitting.current = true;setBusy(true);setError("");
    const controller = new AbortController();request.current = controller;
    try {
      const result = await trackingRequest<DeploymentAction>("deploy-execute",{...selection,token:plan.token},controller.signal);
      if (controller.signal.aborted) return;
      setPlan(null);setNotice(result.warning || result.result.message);
      onRequested(result);
      window.dispatchEvent(new CustomEvent("gitme:deployment-requested",{detail:{projectId,serviceId}}));
    } catch(error) {
      if (!controller.signal.aborted) {setPlan(null);setError(`${error instanceof Error ? error.message : "Deployment confirmation unavailable."} Check Activity and Vercel before retrying.`);}
    } finally {submitting.current = false;if (!controller.signal.aborted) setBusy(false);}
  }
  const canDeploy = release && release.target && !["link-changed","source-changed","unlinked"].includes(release.state);
  return <section className="manual-deployment" aria-label="Manual deployment"><div className="git-push-heading"><div><h3>Manual deployment</h3><p className="git-help">Deploy or rebuild the selected GitHub commit. Local edits are excluded.</p></div><div className="manual-deploy-actions"><label>Target<select aria-label="Manual deployment target" value={target} disabled={busy} onChange={(event) => setTarget(event.target.value as "preview"|"production")}><option value="preview">Preview</option><option value="production">Production</option></select></label><button className="button primary integration-action" disabled={disabled || busy || !canDeploy} onClick={() => {void review();}}><Icon name="upload" size={15}/>{busy ? "Checking..." : "Review deployment"}</button></div></div>
    {!release && <p className="git-help">Track a GitHub commit above first.</p>}
    {release && <p className="git-repository-path"><span>{release.branch}</span><code>{release.commit.slice(0,7)}</code></p>}
    <p className="git-help">With Git auto-deploy enabled, a push usually handles deployment already.</p>
    {notice && <p className="git-feedback" role="status">{notice}</p>}
    {error && <p className="github-error" role="alert">{error}</p>}
    <button className="text-link" disabled={busy} onClick={onActivity}><Icon name="arrow" size={13}/>Open activity</button>
    {plan && <DeploymentReview plan={plan} busy={busy} disabled={disabled} close={close} deploy={() => {void deploy();}}/>}
  </section>;
}

function DeploymentReview({plan,busy,disabled,close,deploy}:{plan:DeploymentPlan;busy:boolean;disabled:boolean;close:()=>void;deploy:()=>void}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [expired,setExpired] = useState(false);
  useEffect(() => {const node = dialog.current;node?.showModal();const timer = setTimeout(() => setExpired(true),Math.max(0,new Date(plan.expires_at).getTime()-Date.now()));return () => {clearTimeout(timer);node?.close();};},[plan]);
  return <dialog ref={dialog} className="dialog git-review" aria-labelledby="manual-deploy-title" onCancel={(event) => {event.preventDefault();if (!busy) close();}}><div className="dialog-heading"><h2 id="manual-deploy-title">Review deployment</h2><button className="icon-button" aria-label="Close deployment review" disabled={busy} onClick={close}><Icon name="close"/></button></div><div className="git-review-body"><dl><dt>Provider</dt><dd>Vercel</dd><dt>Project</dt><dd>{plan.link.scopeSlug}/{plan.link.projectName}</dd><dt>Account</dt><dd>{plan.link.connectionId}</dd><dt>Environment</dt><dd>{plan.target === "production" ? "Production" : "Preview"}</dd><dt>Repository</dt><dd>{plan.repository}</dd><dt>Branch</dt><dd>{plan.branch}</dd><dt>Exact commit</dt><dd><code>{plan.commit}</code></dd></dl><p className="git-notice">{plan.target === "production" ? "A successful Production deployment may replace the version served on your live domain, according to the project's domain settings." : "This creates a Preview deployment without requesting a change to the production domain."}</p><p className="git-help">Vercel uses its saved build settings and environment variables. This requests a fresh build, even if this commit was already deployed. Local files and secrets are not uploaded.</p>{expired && <p className="github-error" role="alert">Review expired. Close it and review again.</p>}</div><div className="dialog-footer"><button className="button integration-action" disabled={busy} onClick={close}>Cancel</button><button className="button primary integration-action" disabled={busy || disabled || expired} onClick={deploy}><Icon name="upload" size={15}/>{busy ? "Submitting..." : plan.target === "production" ? "Confirm production deployment" : "Confirm preview deployment"}</button></div></dialog>;
}
