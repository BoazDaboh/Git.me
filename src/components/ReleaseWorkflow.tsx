"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { updateWorkspace, useWorkspaceStorage } from "@/lib/store";
import { inspectRelease, releaseRoutes, validReleaseConfig, type ReleaseConfig, type ReleaseInspection, type ReleaseRoute } from "@/lib/releases";
import { githubRequest, repoSuggestion, type GitHubRepo } from "@/lib/github";
import { vercelRequest, type VercelActivity, type VercelDeployment } from "@/lib/vercel";
import type { ServiceProfile } from "@/lib/services";
import { useGitHub } from "./GitHubConnection";
import { useVercel } from "./VercelConnection";
import Icon from "./Icon";

type Check = { label: string; state: "ready" | "missing" | "unknown"; detail: string };
type Review = { identity: string; route: ReleaseRoute; inspection: ReleaseInspection; repository: GitHubRepo | null; repositoryError: string; cloud: VercelActivity | null; cloudError: string };

export default function ReleaseWorkflow({ projectId, profile }: { projectId: string; profile: ServiceProfile }) {
  return <WorkflowBody key={JSON.stringify([profile.folder, profile.github, profile.vercel, profile.release])} projectId={projectId} profile={profile}/>;
}

function WorkflowBody({ projectId, profile }: { projectId: string; profile: ServiceProfile }) {
  const storage = useWorkspaceStorage(); const github = useGitHub(); const vercel = useVercel();
  const [config, setConfig] = useState<ReleaseConfig>(profile.release || { route: "github", target: "preview", branch: profile.branch });
  const [inspection, setInspection] = useState<ReleaseInspection | null>(null);
  const [busy, setBusy] = useState(false); const [error, setError] = useState("");
  const [review, setReview] = useState<Review | null>(null);
  const [reviewOpen, setReviewOpen] = useState(false);
  const request = useRef<AbortController | null>(null);
  const dirty = !profile.release || JSON.stringify(config) !== JSON.stringify(profile.release);
  const githubAccount = github.connections.find((item) => !profile.github?.connectionId || profile.github.connectionId === "legacy" ? item.legacy : item.id === profile.github.connectionId);
  const githubReady = github.available && !!githubAccount?.connected && !!profile.github;
  const vercelAccount = vercel.connections.find((item) => item.id === profile.vercel?.connectionId);
  const vercelReady = vercel.available && !!vercelAccount?.connected && !!profile.vercel;
  const identity = JSON.stringify([github.available, githubAccount?.id, githubAccount?.connected, githubAccount?.checked_at, vercel.available, vercelAccount?.id, vercelAccount?.connected, vercelAccount?.checked_at]);
  useEffect(() => () => { request.current?.abort(); }, []);
  function requirements(route: ReleaseRoute, local = inspection): Check[] {
    const checks: Check[] = [];
    if (route !== "vercel-remote") {
      checks.push({ label: "Local service folder", state: !local ? "unknown" : local.folder_ready ? "ready" : "missing", detail: local?.folder_error || local?.folder || (busy ? "Loading service setup..." : "Setup could not be loaded. Retry Refresh setup.") });
      if (route === "github") {
        checks.push({ label: "Git on PATH", state: !local ? "unknown" : local.git_located ? "ready" : "missing", detail: "Executable presence only; authentication is not checked." });
        checks.push({ label: "Local repository", state: !local ? "unknown" : local.git.found && !!local.git.commit && !local.git.detached ? "ready" : "missing", detail: local?.git.detached ? "Detached HEAD. Choose a branch before a future push." : local?.git.commit ? `${local.git.branch || "No branch"} · ${local.git.commit.slice(0, 7)}` : "A readable repository with a commit and branch is required." });
        const remote = local ? repoSuggestion(local.git.repository) : null;
        checks.push({ label: "Repository match", state: !local ? "unknown" : remote && profile.github && remote.toLowerCase() === profile.github.fullName.toLowerCase() ? "ready" : "missing", detail: remote ? `Local origin: ${remote}` : "A GitHub origin matching this service's link is required." });
      } else checks.push({ label: "Vercel CLI on PATH", state: !local ? "unknown" : local.vercel_located ? "ready" : "missing", detail: "Executable presence only; CLI login and upload exclusions are not checked." });
    }
    if (route === "github") checks.push({ label: "Service GitHub connection", state: githubReady ? "ready" : "missing", detail: profile.github?.fullName || "Link this service's GitHub repository below." });
    else checks.push({ label: "Service Vercel connection", state: vercelReady ? "ready" : "missing", detail: profile.vercel ? `${profile.vercel.scopeSlug}/${profile.vercel.projectName}` : "Link this service's Vercel project below." });
    return checks;
  }
  const loadRelease = useCallback(async (controller: AbortController, open = false) => {
    setBusy(true); setError(""); setReviewOpen(open);
    const source = config.route === "github" && profile.github && githubReady ? githubRequest<GitHubRepo>(`repository?${new URLSearchParams({ connection_id: githubAccount!.id, full_name: profile.github.fullName })}`, undefined, controller.signal) : Promise.resolve(null);
    const hosted = profile.vercel && vercelReady ? vercelRequest<VercelActivity>(`activity?${new URLSearchParams({ project_id: projectId, service_id: profile.id })}`, undefined, controller.signal) : Promise.resolve(null);
    const [local, repository, cloud] = await Promise.allSettled([inspectRelease(projectId, profile.id, controller.signal).then((result) => { if (!controller.signal.aborted) setInspection(result); return result; }), source, hosted]);
    if (controller.signal.aborted) return;
    if (local.status === "rejected") setError(local.reason instanceof Error ? local.reason.message : "Service inspection unavailable.");
    else {
      setInspection(local.value);
      setReview({ identity, route: config.route, inspection: local.value, repository: repository.status === "fulfilled" ? repository.value : null, repositoryError: repository.status === "rejected" ? String(repository.reason instanceof Error ? repository.reason.message : "Repository unavailable") : "", cloud: cloud.status === "fulfilled" ? cloud.value : null, cloudError: cloud.status === "rejected" ? String(cloud.reason instanceof Error ? cloud.reason.message : "Vercel unavailable") : "" });
    }
    setBusy(false);
  }, [projectId, profile.id, profile.github, profile.vercel, config.route, githubReady, githubAccount, vercelReady, identity]);
  useEffect(() => {
    if (storage.state !== "saved") return;
    request.current?.abort(); const controller = new AbortController(); request.current = controller;
    const timer = setTimeout(() => { if (!controller.signal.aborted) void loadRelease(controller); }, 0);
    return () => { clearTimeout(timer); controller.abort(); };
  }, [storage.state, loadRelease]);
  function refreshRelease(open = false) {
    request.current?.abort(); const controller = new AbortController(); request.current = controller;
    void loadRelease(controller, open);
  }
  const loaded = review?.identity === identity && review.route === config.route ? review : null;
  const checks = requirements(config.route);
  if (config.route === "github" && loaded && profile.github) checks.push({ label: "Repository read access", state: loaded.repository?.id === profile.github.id ? "ready" : "missing", detail: loaded.repositoryError || loaded.repository?.full_name || "Repository read access could not be verified." });
  if (config.route !== "github" && loaded && profile.vercel) checks.push({ label: "Vercel project read access", state: loaded.cloud ? "ready" : "missing", detail: loaded.cloudError || loaded.cloud?.project.name || "Vercel project read access could not be verified." });
  return <section className="panel release-workflow" aria-label="Service release workflow">
    <div className="panel-heading"><h2><Icon name="flow" size={16}/>Release workflow</h2><span className="badge"><i/>Setup & review</span></div>
    <div className="release-body"><div className="release-routes" role="group" aria-label="Release route">{releaseRoutes.map((route) => {
      const state = requirements(route.id); const incomplete = state.some((item) => item.state === "missing"); const unknown = state.some((item) => item.state === "unknown");
      return <button type="button" disabled={busy} key={route.id} className={`release-route ${config.route === route.id ? "selected" : ""}`} aria-pressed={config.route === route.id} onClick={() => { setConfig((value) => ({ ...value, route: route.id })); setReviewOpen(false); }}><span className="release-route-title"><Icon name={route.id === "github" ? "branch" : route.id === "vercel-local" ? "folder" : "globe"} size={17}/>{route.title}{config.route === route.id && <Icon name="check" size={15}/>}</span><span>{route.description}</span><small>{incomplete ? "Setup required" : unknown ? "Local checks needed" : "Available for review"}</small></button>;
    })}</div>
      <div className="release-fields"><label>Intended environment<select aria-label="Intended release environment" disabled={busy} value={config.target} onChange={(event) => { setConfig((value) => ({ ...value, target: event.target.value as ReleaseConfig["target"] })); setReviewOpen(false); }}><option value="preview">Preview</option><option value="production">Production</option></select></label>{config.route === "github" && <label>Branch to push<input disabled={busy} value={config.branch} maxLength={100} onChange={(event) => { setConfig((value) => ({ ...value, branch: event.target.value })); setReviewOpen(false); }} placeholder="Choose your branch"/></label>}</div>
      {config.route === "github" && <p className="release-help">Git deployments follow the hosting provider&apos;s branch rules. The selected environment does not override them.</p>}
      <ol className="release-steps">{releaseRoutes.find((route) => route.id === config.route)?.steps.map((step) => <li key={step}>{step}</li>)}</ol>
      {busy && <p className="release-help" role="status">Loading release data...</p>}<div className="release-checks">{checks.map((check) => <CheckRow key={check.label} check={check} loading={busy}/>)}</div>
      {inspection && <p className="release-help">Local setup checked {new Date(inspection.checked_at).toLocaleTimeString("en-GB")}</p>}
      <div className="release-actions"><button className="button integration-action" disabled={busy || storage.state !== "saved"} onClick={() => { refreshRelease(); }}><Icon name="search" size={15}/>{busy ? "Checking..." : "Refresh setup"}</button><button className={`button integration-action ${dirty ? "primary" : ""}`} disabled={busy || !dirty || storage.state !== "saved"} onClick={() => {
        if (!validReleaseConfig(config)) { setError("Choose a valid workflow, environment and branch."); return; }
        updateWorkspace((workspace) => ({ ...workspace, projects: workspace.projects.map((project) => project.id !== projectId ? project : { ...project, services: (project.services ?? []).map((item) => item.id === profile.id ? { ...item, release: config } : item) }) }));
      }}><Icon name="check" size={15}/>Save workflow</button><button className="button primary integration-action" disabled={busy || dirty || storage.state !== "saved"} onClick={() => { refreshRelease(true); }}><Icon name="flow" size={16}/>Review release</button></div>
      {dirty && <p className="release-help">Save this service&apos;s workflow before reviewing.</p>}{storage.state !== "saved" && <p className="release-help">Wait for Saved to SQLite.</p>}{error && <p className="github-error" role="alert">{error}</p>}
    </div>
    {reviewOpen && review && review.identity === identity && storage.state === "saved" && <ReleaseReview profile={profile} config={config} review={review} checks={requirements(config.route, review.inspection)} githubAccount={githubAccount?.account?.login || "Not connected"} vercelAccount={vercelAccount?.username || "Not connected"} busy={busy} refresh={() => { refreshRelease(true); }} close={() => { setReviewOpen(false); }}/>}
  </section>;
}

function CheckRow({ check, loading = false }: { check: Check; loading?: boolean }) { return <div className="release-check" data-state={check.state}><span><Icon name={check.state === "ready" ? "check" : check.state === "missing" ? "close" : "clock"} size={15}/><strong>{check.label}</strong><small>{check.state === "ready" ? "Detected" : check.state === "missing" ? "Needs setup" : loading ? "Checking..." : "Not verified"}</small></span><p>{check.detail}</p></div>; }

function ReleaseReview({ profile, config, review, checks, githubAccount, vercelAccount, busy, refresh, close }: { profile: ServiceProfile; config: ReleaseConfig; review: Review; checks: Check[]; githubAccount: string; vercelAccount: string; busy: boolean; refresh: () => void; close: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null); const [sourceId, setSourceId] = useState("");
  useEffect(() => { const node = dialog.current; node?.showModal(); return () => node?.close(); }, []);
  const versions: VercelDeployment[] = [...(review.cloud?.production ? [review.cloud.production] : []), ...(review.cloud?.deployments || [])].filter((item, index, items) => items.findIndex((entry) => entry.id === item.id) === index);
  const selected = versions.find((item) => item.id === sourceId);
  const verified = [...checks];
  if (config.route === "github") {
    verified.push({ label: "Repository access", state: review.repository?.id === profile.github?.id && !!review.repository ? "ready" : "missing", detail: review.repositoryError || review.repository?.full_name || "Repository read access could not be verified." });
    verified.push({ label: "Requested branch", state: config.branch && config.branch === review.inspection.git.branch ? "ready" : "missing", detail: `Requested: ${config.branch || "Not set"}. Local: ${review.inspection.git.branch || "Not detected"}.` });
    verified.push({ label: "Push permission", state: "unknown", detail: "The read-only account connection does not establish local Git push permission." });
    verified.push({ label: "Automatic deployment", state: "unknown", detail: "Git deployment rules are not verified. A push could update production or trigger no deployment." });
  } else {
    verified.push({ label: "Vercel project access", state: review.cloud ? "ready" : "missing", detail: review.cloudError || review.cloud?.project.name || "Vercel project read access could not be verified." });
    verified.push({ label: config.route === "vercel-remote" ? "Source version" : "Upload contents", state: config.route === "vercel-remote" && selected ? "ready" : "unknown", detail: config.route === "vercel-remote" ? selected ? `${selected.id}${selected.sha ? ` · ${selected.sha.slice(0, 7)}` : " · Commit SHA not reported"}` : "Choose a deployment already available on Vercel." : "Local changes and upload exclusions have not been reviewed." });
    verified.push({ label: "Deployment permission", state: "unknown", detail: "Read access does not establish deployment permission." });
  }
  return <dialog ref={dialog} className="dialog release-review" aria-labelledby="release-review-title" onCancel={(event) => { event.preventDefault(); close(); }}>
    <div className="dialog-heading"><div><h2 id="release-review-title">Review release</h2><p>{profile.name} · Review only</p></div><button className="icon-button" aria-label="Close release review" onClick={close}><Icon name="close"/></button></div>
    <div className="release-review-body"><dl><dt>Workflow</dt><dd>{releaseRoutes.find((route) => route.id === config.route)?.title}</dd><dt>Intended environment</dt><dd>{config.target}</dd><dt>Local folder</dt><dd><code>{review.inspection.folder || "Not required for remote deployment"}</code></dd>{config.route === "github" ? <><dt>GitHub account</dt><dd>{githubAccount}</dd><dt>Repository</dt><dd>{profile.github?.fullName || "Not linked"}</dd><dt>Local HEAD</dt><dd><code>{review.inspection.git.commit || "Not detected"}</code></dd></> : <><dt>Vercel account</dt><dd>{vercelAccount}</dd></>}
      <dt>Vercel project</dt><dd>{profile.vercel ? `${profile.vercel.scopeSlug}/${profile.vercel.projectName}` : "Not linked"}</dd><dt>Review time</dt><dd>{new Date(review.inspection.checked_at).toLocaleString("en-GB")}</dd></dl>
      {config.route === "vercel-remote" && <label>Source deployment<select aria-label="Source deployment" value={sourceId} onChange={(event) => setSourceId(event.target.value)}><option value="">Choose an existing deployment</option>{versions.map((item) => <option key={item.id} value={item.id}>{item.id} · {item.state.toLowerCase()} · {item.sha?.slice(0, 7) || "SHA unavailable"}</option>)}</select></label>}
      <div className="release-checks">{verified.map((check) => <CheckRow key={check.label} check={check}/>)}</div>
      <p className="release-review-notice">Review only. No files are staged, no commits are pushed, and no deployments are started.</p>
      {config.route === "github" && config.target === "production" && <p className="release-help">A future push to the production branch may publish immediately.</p>}
      <Link className="button integration-secondary" href="/settings" onClick={close}><Icon name="link" size={15}/>Manage connections</Link>
    </div><div className="dialog-footer"><button className="button integration-action" onClick={close}>Close</button><button className="button primary integration-action" disabled={busy} onClick={refresh}><Icon name="refresh" size={15}/>{busy ? "Checking..." : "Refresh review"}</button></div>
  </dialog>;
}





