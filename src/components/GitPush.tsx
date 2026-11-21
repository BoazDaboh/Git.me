"use client";

import { useEffect, useRef, useState } from "react";
import { gitRequest, gitPushTerminalCommand, type GitPushPlan, type GitPushResult } from "@/lib/local-git";
import type { ServiceProfile } from "@/lib/services";
import GitCommand from "./GitCommand";
import Icon from "./Icon";

export default function GitPush({ projectId, profile, disabled, onPushed }: { projectId: string; profile: ServiceProfile; disabled: boolean; onPushed: () => void }) {
  const [plan, setPlan] = useState<GitPushPlan | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<GitPushResult | null>(null);
  const [message, setMessage] = useState("");
  const request = useRef<AbortController | null>(null);
  const livePlan = useRef<GitPushPlan | null>(null);
  const selection = { project_id: projectId, service_id: profile.id };
  useEffect(() => { livePlan.current = plan; }, [plan]);
  useEffect(() => () => {
    request.current?.abort();
    if (livePlan.current?.token) void gitRequest("push-cancel", { project_id: projectId, service_id: profile.id, token: livePlan.current.token }).catch(() => {});
  }, [projectId, profile.id]);
  async function review() {
    const controller = new AbortController(); request.current = controller;
    setBusy(true); setError(""); setMessage(""); setResult(null);
    try {
      const next = await gitRequest<GitPushPlan>("push-review", selection, controller.signal);
      if (controller.signal.aborted) return;
      if (next.commits.length === 0) setMessage(`${next.branch} is up to date with GitHub at ${next.head.slice(0, 7)}.`);
      else setPlan(next);
    } catch (error) { if (!controller.signal.aborted) setError(error instanceof Error ? error.message : "Push review unavailable."); }
    finally { if (!controller.signal.aborted) setBusy(false); }
  }
  function close() {
    if (!plan || busy) return;
    void gitRequest("push-cancel", { ...selection, token: plan.token }).catch(() => {});
    setPlan(null);
  }
  async function push() {
    if (!plan || busy || disabled) return;
    const controller = new AbortController(); request.current = controller; setBusy(true); setError("");
    try {
      const pushed = await gitRequest<GitPushResult>("push-execute", { ...selection, token: plan.token }, controller.signal);
      if (controller.signal.aborted) return;
      setResult(pushed); setPlan(null); onPushed();
      window.dispatchEvent(new CustomEvent("gitme:git-pushed", { detail: { projectId, serviceId: profile.id } }));
    } catch (error) { if (!controller.signal.aborted) { setPlan(null); setError(error instanceof Error ? error.message : "Push was not confirmed. Review the branch before retrying."); } }
    finally { if (!controller.signal.aborted) setBusy(false); }
  }
  return <section className="git-push" aria-label="Push to GitHub"><div className="git-push-heading"><div><h3>Push to GitHub</h3><p className="git-help">Upload committed changes using your PC&apos;s Git Credential Manager sign-in.</p></div><button className="button primary integration-action" disabled={disabled || busy || !profile.folder.trim()} onClick={() => { void review(); }}><Icon name="upload" size={15}/>{busy ? "Checking GitHub..." : "Review push"}</button></div>
    <p className="git-repository-path"><strong>{profile.name}</strong><code>{profile.folder || "Set this service's local folder first."}</code></p>
    {message && <p className="git-feedback" role="status">{message}</p>}
    {result && <div className="git-push-result" role="status"><p>{result.message}</p><a className="button integration-secondary" href={`${result.url.replace(/\.git$/, "")}/commit/${result.commit}`} target="_blank" rel="noreferrer"><Icon name="external" size={14}/>View pushed commit</a><p className="git-help">{result.tracking_warning || "Follow this commit in Deployment tracking below."}</p></div>}
    {error && <p className="github-error" role="alert">{error}</p>}
    {plan && <PushReview plan={plan} busy={busy} disabled={disabled} close={close} push={() => { void push(); }}/>} 
  </section>;
}

function PushReview({ plan, busy, disabled, close, push }: { plan: GitPushPlan; busy: boolean; disabled: boolean; close: () => void; push: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [expired, setExpired] = useState(false);
  useEffect(() => {
    const node = dialog.current; node?.showModal();
    const timer = setTimeout(() => setExpired(true), Math.max(0, new Date(plan.expires_at).getTime() - Date.now()));
    return () => { clearTimeout(timer); node?.close(); };
  }, [plan]);
  return <dialog ref={dialog} className="dialog git-review" aria-labelledby="git-push-title" onCancel={(event) => { event.preventDefault(); if (!busy) close(); }}><div className="dialog-heading"><h2 id="git-push-title">Review push to GitHub</h2><button className="icon-button" aria-label="Close push review" disabled={busy} onClick={close}><Icon name="close"/></button></div>
    <div className="git-review-body"><dl><dt>Destination</dt><dd>{plan.url}</dd><dt>Branch</dt><dd>{plan.branch} → {plan.branch}{!plan.remote_head && " (new GitHub branch)"}</dd><dt>Local commit</dt><dd><code>{plan.head}</code></dd><dt>GitHub commit</dt><dd><code>{plan.remote_head || "No branch yet"}</code></dd><dt>Sign-in</dt><dd>Your PC&apos;s Git Credential Manager. The connected dashboard account is read-only.</dd></dl>
      <h3>{plan.commits.length} outgoing commit{plan.commits.length === 1 ? "" : "s"}</h3><ol className="git-push-commits">{plan.commits.map((commit) => <li key={commit.sha}><code>{commit.sha.slice(0, 7)}</code><span>{commit.title}</span></li>)}</ol>
      <details className="git-push-files"><summary>{plan.files.length} file path{plan.files.length === 1 ? "" : "s"} changed in outgoing history</summary><ul className="git-reviewed-files">{plan.files.map((path) => <li key={path}>{path}</li>)}</ul></details>
      <p className="git-notice">This pushes committed history for the whole repository, including other services. A push may trigger an automatic deployment, including production, according to your provider&apos;s branch rules.</p>
      {plan.uncommitted > 0 && <p className="git-help">Uncommitted changes stay on your PC and are not included.</p>}
      <GitCommand title="Push command · PowerShell" command={gitPushTerminalCommand(plan)} description="Pushes this exact commit to this branch without hooks or extra tags. Running it in your terminal bypasses Git/Me's review checks."/>
      {expired && <p className="github-error" role="alert">This push review expired. Close it and review again.</p>}
    </div><div className="dialog-footer"><button className="button integration-action" disabled={busy} onClick={close}>Cancel</button><button className="button primary integration-action" disabled={busy || disabled || expired} onClick={push}><Icon name="upload" size={15}/>{busy ? "Pushing..." : "Confirm push"}</button></div>
  </dialog>;
}
