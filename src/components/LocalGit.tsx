"use client";

import { useEffect, useRef, useState } from "react";
import { useWorkspaceStorage } from "@/lib/store";
import { gitRequest, gitTerminalCommand, hasStaged, hasWorking, type GitStatus, type GitDiff, type GitPlan, type GitResult, type GitAction } from "@/lib/local-git";
import type { ServiceProfile } from "@/lib/services";
import Icon from "./Icon";
import GitCommand from "./GitCommand";
import GitPush from "./GitPush";

export default function LocalGit({ projectId, profile }: { projectId: string; profile: ServiceProfile }) {
  return <GitBody key={`${projectId}:${profile.id}:${profile.folder}`} projectId={projectId} profile={profile}/>;
}

function GitBody({ projectId, profile }: { projectId: string; profile: ServiceProfile }) {
  const storage = useWorkspaceStorage();
  const [status, setStatus] = useState<GitStatus | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [diff, setDiff] = useState<GitDiff | null>(null);
  const [plan, setPlan] = useState<GitPlan | null>(null);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [feedback, setFeedback] = useState("");
  const request = useRef<AbortController | null>(null);
  const livePlan = useRef<GitPlan | null>(null);
  const selection = { project_id: projectId, service_id: profile.id };
  useEffect(() => { livePlan.current = plan; }, [plan]);
  useEffect(() => () => {
    request.current?.abort();
    if (livePlan.current) void gitRequest("cancel", { project_id: projectId, service_id: profile.id, token: livePlan.current.token }).catch(() => {});
  }, [projectId, profile.id]);
  async function refresh() {
    request.current?.abort(); const controller = new AbortController(); request.current = controller; setBusy(true); setError(""); setFeedback(""); setDiff(null); setSelected([]);
    try { const result = await gitRequest<GitStatus>("status", selection, controller.signal); if (!controller.signal.aborted) setStatus(result); }
    catch (error) { if (!controller.signal.aborted) { setStatus(null); setError(error instanceof Error ? error.message : "Git status unavailable."); } }
    finally { if (!controller.signal.aborted) setBusy(false); }
  }
  async function showDiff(path: string, staged: boolean) {
    if (!status) return;
    const controller = new AbortController(); request.current = controller; setBusy(true); setError(""); setDiff(null);
    try { const result = await gitRequest<GitDiff>("diff", { ...selection, version: status.version, path, staged }, controller.signal); if (!controller.signal.aborted) setDiff(result); }
    catch (error) { if (!controller.signal.aborted) setError(error instanceof Error ? error.message : "Diff unavailable."); }
    finally { if (!controller.signal.aborted) setBusy(false); }
  }
  async function prepare(action: GitAction) {
    if (!status) return;
    const paths = action === "commit" ? [] : status.files.filter((file) => selected.includes(file.path) && !file.blocked && (action === "stage" ? hasWorking(file) : hasStaged(file))).map((file) => file.path);
    const controller = new AbortController(); request.current = controller; setBusy(true); setError(""); setFeedback("");
    try { const result = await gitRequest<GitPlan>("prepare", { ...selection, version: status.version, action, message: action === "commit" ? message : "", paths }, controller.signal); if (!controller.signal.aborted) setPlan(result); }
    catch (error) { if (!controller.signal.aborted) setError(error instanceof Error ? error.message : "Git review unavailable."); }
    finally { if (!controller.signal.aborted) setBusy(false); }
  }
  function closeReview() { if (!plan || busy) return; void gitRequest("cancel", { ...selection, token: plan.token }).catch(() => {}); setPlan(null); }
  async function execute() {
    if (!plan) return;
    const reviewed = plan; const controller = new AbortController(); request.current = controller; setBusy(true); setError("");
    try {
      const result = await gitRequest<GitResult>("execute", { ...selection, token: reviewed.token }, controller.signal);
      if (controller.signal.aborted) return;
      setStatus(result.status); setSelected([]); setDiff(null); setPlan(null);
      if (result.action === "commit") { setMessage(""); setFeedback(`Local commit ${result.commit?.slice(0, 7)} saved on ${result.branch}. Nothing was pushed.`); }
      else setFeedback(`${reviewed.files.length} file${reviewed.files.length === 1 ? "" : "s"} ${result.action === "stage" ? "staged" : "unstaged"}.`);
    } catch (error) {
      if (!controller.signal.aborted) { setPlan(null); setError(error instanceof Error ? error.message : "Git operation failed. Refresh before retrying."); try { setStatus(await gitRequest<GitStatus>("status", selection, controller.signal)); setSelected([]); setDiff(null); } catch {} }
    } finally { if (!controller.signal.aborted) setBusy(false); }
  }
  const stageCount = status?.files.filter((file) => selected.includes(file.path) && !file.blocked && hasWorking(file)).length || 0;
  const unstageCount = status?.files.filter((file) => selected.includes(file.path) && !file.blocked && hasStaged(file)).length || 0;
  const staged = status?.files.filter(hasStaged) || [];
  const stagePaths = status?.files.filter((file) => selected.includes(file.path) && !file.blocked && hasWorking(file)).map((file) => file.path) || [];
  const unstagePaths = status?.files.filter((file) => selected.includes(file.path) && !file.blocked && hasStaged(file)).map((file) => file.path) || [];
  const available = storage.state === "saved" && !busy;
  const selectablePaths = status?.files.filter((file) => !file.blocked).map((file) => file.path) || [];
  const selectedCount = selectablePaths.filter((path) => selected.includes(path)).length;
  const allSelected = selectablePaths.length > 0 && selectedCount === selectablePaths.length;
  return <section className="panel local-git" aria-label="Service local Git">
    <div className="panel-heading"><h2><Icon name="branch" size={16}/>Local Git</h2><button className="button integration-action" disabled={!available} onClick={() => { void refresh(); }}><Icon name="refresh" size={15}/>{busy ? "Working..." : "Refresh Git status"}</button></div>
    <div className="local-git-body">
      {!status && !error && <p className="git-help">Load this service&apos;s changed files, then choose what to stage.</p>}
      {status && <><div className="git-summary"><span><Icon name="branch" size={14}/>{status.branch || "Detached HEAD"}</span><code>{status.head ? status.head.slice(0, 7) : "No commits yet"}</code><span>{status.files.length} changed · {staged.length} staged</span></div><p className="git-repository-path"><strong>Repository</strong><code>{status.repository}</code></p>
        {status.files.length > 0 && <div className="git-selection-bar"><label className="git-file-select"><input type="checkbox" aria-label="Select all files" checked={allSelected} disabled={busy || selectablePaths.length === 0} ref={(node) => { if (node) node.indeterminate = selectedCount > 0 && !allSelected; }} onChange={(event) => setSelected(event.target.checked ? selectablePaths : [])}/><span>Select all</span></label><span>{selectedCount} selected</span></div>}
        {status.files.length ? <div className="git-files" role="group" aria-label="Changed files">{status.files.map((file) => <div className={`git-file ${file.blocked ? "blocked" : ""}`} key={file.path}>
          <label className="git-file-select"><input type="checkbox" aria-label={`Select ${file.path}`} disabled={busy || !!file.blocked} checked={selected.includes(file.path)} onChange={(event) => setSelected((items) => event.target.checked ? [...items, file.path] : items.filter((path) => path !== file.path))}/><span>{file.path}</span></label>
          <div className="git-file-actions">{hasStaged(file) && <span className="git-state">Staged: {statusLabel(file.index)}</span>}{hasWorking(file) && <span className="git-state">Working: {statusLabel(file.worktree)}</span>}{!file.blocked && hasWorking(file) && <button className="button" disabled={busy} onClick={() => { void showDiff(file.path, false); }}>Working diff</button>}{!file.blocked && hasStaged(file) && <button className="button" disabled={busy} onClick={() => { void showDiff(file.path, true); }}>Staged diff</button>}</div>{file.blocked && <p>{file.blocked}</p>}
        </div>)}</div> : <p className="git-help">No changed files in this service.</p>}
        {status.outside_staged > 0 && <p className="git-notice">{status.outside_staged} staged path(s) belong outside this service. Use the repository root or terminal for that commit.</p>}
        <div className="git-actions"><button className="button primary integration-action" disabled={!available || stageCount === 0} onClick={() => { void prepare("stage"); }}><Icon name="plus" size={15}/>Review stage{stageCount ? ` (${stageCount})` : ""}</button><button className="button integration-action" disabled={!available || unstageCount === 0} onClick={() => { void prepare("unstage"); }}><Icon name="close" size={15}/>Review unstage{unstageCount ? ` (${unstageCount})` : ""}</button></div>
        <details className="git-terminal"><summary><Icon name="terminal" size={15}/>Terminal commands · PowerShell</summary>
          <p className="git-help">Copy to run manually. -C chooses this repository; --literal-pathspecs treats file names exactly as written. Refresh Git status afterwards.</p>
          <GitCommand title="Check status" command={gitTerminalCommand(status.repository, "status")} description="Shows your current branch, staged and unstaged changes."/>
          <GitCommand title="Stage selected files" command={gitTerminalCommand(status.repository, "stage", stagePaths)} description="git add selects the current file changes for your next commit." empty="Select changed files above to see their stage command."/>
          <GitCommand title="Unstage selected files" command={gitTerminalCommand(status.repository, "unstage", unstagePaths, "", !!status.head)} description={status.head ? "git restore --staged removes changes from staging and keeps your working files." : "Before the first commit, git rm --cached removes entries from staging and keeps your working files."} empty="Select staged files above to see their unstage command."/>
          <GitCommand title="Commit staged changes" command={gitTerminalCommand(status.repository, "commit", [], message)} description="git commit saves all staged changes locally, including any staged outside this service. Terminal commits use your configured hooks and signing." empty="Write a commit message below to see its command."/>
        </details>
        {diff && <section className="git-diff-view" aria-label="File diff"><div><h3>{diff.path}</h3><span>{diff.staged ? "Staged content" : "Working changes"}</span><button className="icon-button" aria-label="Close diff" onClick={() => setDiff(null)}><Icon name="close" size={15}/></button></div>{diff.binary ? <p className="git-help">Binary content cannot be previewed.</p> : <Patch text={diff.text || "No text changes returned."}/>}{diff.truncated && <p className="git-help">Preview limited to 64 KiB.</p>}</section>}
        <div className="git-commit"><label>Commit message<textarea value={message} maxLength={2000} rows={3} disabled={busy} placeholder="Describe the staged change" onChange={(event) => setMessage(event.target.value)}/></label><button className="button primary integration-action" disabled={!available || !message.trim() || staged.length === 0 || staged.some((file) => !!file.blocked) || status.outside_staged > 0 || !status.branch} onClick={() => { void prepare("commit"); }}><Icon name="check" size={15}/>Review commit ({staged.length})</button><p className="git-help">Commits contain every staged file in this service, including files staged from your terminal.</p></div>
      </>}
      {storage.state !== "saved" && <p className="git-help">Wait for Saved to SQLite before using Git controls.</p>}{feedback && <p className="git-feedback" role="status">{feedback}</p>}{error && <p className="github-error" role="alert">{error}</p>}
      <GitPush key={JSON.stringify([profile.repository, profile.github])} projectId={projectId} profile={profile} disabled={!available} onPushed={() => { void refresh(); }}/>
    </div>
    {plan && status && <GitReview plan={plan} repository={status.repository} hasHead={!!status.head} busy={busy} close={closeReview} execute={() => { void execute(); }}/>}
  </section>;
}

function GitReview({ plan, repository, hasHead, busy, close, execute }: { plan: GitPlan; repository: string; hasHead: boolean; busy: boolean; close: () => void; execute: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null); const [expired, setExpired] = useState(false);
  useEffect(() => { const node = dialog.current; node?.showModal(); const timer = setTimeout(() => setExpired(true), Math.max(0, new Date(plan.expires_at).getTime() - Date.now())); return () => { clearTimeout(timer); node?.close(); }; }, [plan]);
  const label = plan.action === "commit" ? "Commit staged changes" : plan.action === "stage" ? "Stage selected files" : "Unstage selected files";
  return <dialog ref={dialog} className="dialog git-review" aria-labelledby="git-review-title" onCancel={(event) => { event.preventDefault(); if (!busy) close(); }}>
    <div className="dialog-heading"><h2 id="git-review-title">Review {plan.action}</h2><button className="icon-button" aria-label="Close Git review" disabled={busy} onClick={close}><Icon name="close"/></button></div>
    <div className="git-review-body"><dl><dt>Branch</dt><dd>{plan.branch}</dd>{plan.author && <><dt>Author</dt><dd>{plan.author}</dd></>}{plan.message && <><dt>Message</dt><dd>{plan.message}</dd></>}</dl><h3>{plan.files.length} reviewed file{plan.files.length === 1 ? "" : "s"}</h3><ul className="git-reviewed-files">{plan.files.map((path) => <li key={path}>{path}</li>)}</ul><Patch text={plan.diff || "No text diff available."}/>{plan.truncated && <p className="git-help">Diff preview is limited to 64 KiB. The operation applies to all reviewed files.</p>}
      <GitCommand title="Terminal equivalent · PowerShell" command={gitTerminalCommand(repository, plan.action, plan.files, plan.message, hasHead)} description={plan.action === "stage" ? "git add stages these files as they exist when you run it. Git/Me stages the reviewed snapshot." : plan.action === "unstage" ? "Removes these files from staging. Working files are kept." : "Commits all staged changes at run time. Terminal commits use your configured hooks and signing."}/>
      <p className="git-notice">{plan.action === "stage" ? "Stages this reviewed snapshot. Working files stay unchanged." : plan.action === "unstage" ? "Removes these changes from staging. Working files stay unchanged." : "Creates a local commit from the reviewed staging area. Hooks and commit signing are not run. Nothing is pushed or deployed."}</p>{expired && <p className="git-help" role="alert">This review expired. Close it and review again.</p>}
    </div><div className="dialog-footer"><button className="button integration-action" disabled={busy} onClick={close}>Cancel</button><button className="button primary integration-action" disabled={busy || expired} onClick={execute}><Icon name="check" size={15}/>{busy ? "Saving..." : label}</button></div>
  </dialog>;
}

function Patch({ text }: { text: string }) { return <pre className="git-patch" tabIndex={0}>{text.split("\n").map((line, index) => <span key={index} className={line.startsWith("+") ? "added" : line.startsWith("-") ? "removed" : line.startsWith("@@") ? "hunk" : ""}>{line}{"\n"}</span>)}</pre>; }
function statusLabel(value: string) { return ({ M: "modified", A: "added", D: "deleted", R: "renamed", C: "copied", T: "type changed", U: "conflict", "?": "untracked" } as Record<string, string>)[value] || "changed"; }

