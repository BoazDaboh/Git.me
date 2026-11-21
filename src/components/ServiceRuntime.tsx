"use client";

import { useEffect, useRef, useState } from "react";
import { useWorkspaceStorage } from "@/lib/store";
import { downloadText } from "@/lib/workspace";
import { processRequest, running, type EndpointHealth, type LaunchPlan, type Run } from "@/lib/processes";
import type { ServiceProfile } from "@/lib/services";
import Icon from "./Icon";

const labels: Record<Run["state"], string> = { not_managed: "Not managed", running: "Running", stopping: "Stopping", stopped: "Stopped", failed: "Failed", exited: "Exited" };
function time(value: string | null) { return value && !value.startsWith("0001") ? new Date(value).toLocaleTimeString("en-GB") : "Not available"; }

export default function ServiceRuntime({ projectId, profile, onActiveChange }: { projectId: string; profile: ServiceProfile; onActiveChange: (active: boolean) => void }) {
  const storage = useWorkspaceStorage();
  const [run, setRun] = useState<Run | null>(null);
  const [available, setAvailable] = useState(false);
  const [connectionError, setConnectionError] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [plan, setPlan] = useState<LaunchPlan | null>(null);
  const [health, setHealth] = useState<EndpointHealth | null>(null);
  const [follow, setFollow] = useState(true);
  const refresh = useRef<() => Promise<void>>(async () => {});
  const log = useRef<HTMLPreElement>(null);
  const active = running(run);
  const selection = { project_id: projectId, service_id: profile.id };
  useEffect(() => onActiveChange(active), [active, onActiveChange]);
  useEffect(() => {
    let mounted = true, checking = false, hasActiveRun = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let controller: AbortController | null = null;
    const check = async () => {
      if (checking || !mounted) return;
      checking = true; controller = new AbortController();
      try { const result = await processRequest<Run>(`/services/status?${new URLSearchParams({ project_id: projectId, service_id: profile.id })}`, undefined, controller.signal); if (mounted) { hasActiveRun = running(result); setRun(result); setAvailable(true); setConnectionError(""); } }
      catch (error) { if (mounted) { setAvailable(false); setConnectionError(error instanceof Error ? error.message : "Process status unavailable."); } }
      finally { checking = false; if (mounted) { clearTimeout(timer); timer = setTimeout(() => { if (document.visibilityState === "visible") void check(); else { clearTimeout(timer); timer = setTimeout(poll, 30000); } }, hasActiveRun ? 2000 : 30000); } }
    };
    refresh.current = check;
    const poll = () => { if (document.visibilityState === "visible") void check(); };
    void check();
    window.addEventListener("focus", poll); document.addEventListener("visibilitychange", poll);
    return () => { mounted = false; clearTimeout(timer); controller?.abort(); window.removeEventListener("focus", poll); document.removeEventListener("visibilitychange", poll); };
  }, [projectId, profile.id]);
  useEffect(() => { if (follow && log.current) log.current.scrollTop = log.current.scrollHeight; }, [run?.logs, follow]);
  async function review(action: "start" | "restart") {
    setBusy(true); setError("");
    try { setPlan(await processRequest<LaunchPlan>("/services/prepare", { ...selection, action })); }
    catch (error) { setError(error instanceof Error ? error.message : "Could not prepare this command."); }
    finally { setBusy(false); }
  }
  async function stop() {
    if (!run?.run_id) return;
    setBusy(true); setError(""); setHealth(null);
    try { setRun(await processRequest<Run>("/services/stop", { ...selection, run_id: run.run_id })); }
    catch (error) { setError(error instanceof Error ? error.message : "Stop failed. Refresh status."); }
    finally { setBusy(false); void refresh.current(); }
  }
  return <section className="panel service-runtime" aria-label="Local process controls"><div className="panel-heading"><h2><Icon name="terminal" size={16}/>Local development</h2><span className={`badge ${available && run?.state === "running" ? "blue" : ""}`} role="status"><i/>{available && run ? labels[run.state] : connectionError ? "Status unavailable" : "Checking"}</span></div><div className="runtime-body"><div className="runtime-actions"><button className="button primary" disabled={busy || !available || active || storage.state !== "saved" || !profile.commands.develop} onClick={() => { void review("start"); }}><Icon name="play" size={14}/>Start</button><button className="button" disabled={busy || !available || run?.state !== "running" || storage.state !== "saved"} onClick={() => { void review("restart"); }}>Restart</button><button className="button" disabled={busy || !available || !active || run?.state === "stopping"} onClick={() => { void stop(); }}><Icon name="pause" size={14}/>Stop</button><button className="text-link" disabled={busy} onClick={() => { void refresh.current(); }}>Refresh status</button></div><p className="runtime-scope">{!available ? connectionError || "Checking the companion." : active ? "Running in the background. Closing this page does not stop it." : "Terminal-started servers are not controlled by Git/Me."}</p>{storage.state !== "saved" && <p className="runtime-scope">Wait for Saved to SQLite before starting or restarting.</p>}{!profile.commands.develop && <p className="runtime-scope">Set a Development command in Edit profile.</p>}{error && <p className="runtime-error" role="alert">{error}</p>}
    {run?.run_id && <><dl className="runtime-readings"><div><dt>Launch PID</dt><dd>{run.pid}</dd></div><div><dt>Started</dt><dd>{time(run.started_at)}</dd></div><div><dt>Ended</dt><dd>{time(run.ended_at)}</dd></div><div><dt>Exit code</dt><dd>{run.exit_code ?? "Not available"}</dd></div></dl><div className="runtime-command"><small>Launched from</small><code>{run.folder}</code><code>{run.command}</code></div>{run.error && <p className="runtime-error">{run.error}</p>}</>}
    <div className="runtime-health"><button className="button" disabled={busy || !available || storage.state !== "saved"} onClick={async () => { setBusy(true); setError(""); try { setHealth(await processRequest<EndpointHealth>("/services/health", selection)); } catch (error) { setError(error instanceof Error ? error.message : "Health check failed."); } finally { setBusy(false); } }}>Check endpoint</button><div role="status">{health ? <><strong>{health.message} · {health.duration_ms} ms</strong><code>{health.url}</code></> : <span>HTTP response is checked separately from process ownership.</span>}</div></div>
    <div className="runtime-log-heading"><h3>Output & errors</h3><div><label><input type="checkbox" checked={follow} onChange={(event) => setFollow(event.target.checked)}/>Follow</label><button className="text-link" disabled={!run?.logs} onClick={() => downloadText(`${profile.name.replace(/[^a-z0-9_-]/gi, "-")}-logs.txt`, run?.logs || "")}><Icon name="download" size={13}/>Export logs</button></div></div><pre ref={log} className="runtime-log" aria-label="Service logs" tabIndex={0}>{run?.logs || (active ? "Waiting for output..." : "Logs appear after a launch.")}</pre>{run?.truncated && <p className="runtime-scope">Showing the most recent 64 KiB. Earlier output was discarded.</p>}
    </div>{plan && <LaunchReview plan={plan} busy={busy} close={() => { if (!busy) setPlan(null); }} execute={async () => {
      setBusy(true); setError(""); setHealth(null);
      try { setRun(await processRequest<Run>("/services/run", { token: plan.token })); setAvailable(true); setPlan(null); }
      catch (error) { setError(error instanceof Error ? error.message : "Launch failed. Refresh status."); setPlan(null); }
      finally { setBusy(false); void refresh.current(); }
    }}/>}</section>;
}

function LaunchReview({ plan, busy, close, execute }: { plan: LaunchPlan; busy: boolean; close: () => void; execute: () => Promise<void> }) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => { const node = dialog.current; node?.showModal(); return () => node?.close(); }, []);
  return <dialog ref={dialog} className="dialog runtime-review" aria-labelledby="runtime-review-title" onCancel={(event) => { event.preventDefault(); close(); }}><div className="dialog-heading"><h2 id="runtime-review-title">{plan.action === "restart" ? "Review restart" : "Review launch"}</h2><button className="icon-button" disabled={busy} aria-label="Close launch review" onClick={close}><Icon name="close"/></button></div><div className="runtime-review-body"><h3>{plan.name}</h3><dl><dt>Working folder</dt><dd><code>{plan.folder}</code></dd><dt>Development command</dt><dd><code>{plan.command}</code></dd><dt>Shell</dt><dd>Windows Command Prompt</dd><dt>Configured port</dt><dd>{plan.port ?? "Not set"}</dd></dl><p>Runs with your Windows user permissions. Use commands from projects you trust.</p>{plan.action === "restart" && <p>Restart stops the current owned process tree, then launches this command.</p>}</div><div className="dialog-footer"><button className="button" disabled={busy} onClick={close}>Cancel</button><button className="button primary" disabled={busy} onClick={() => { void execute(); }}>{busy ? "Working..." : plan.action === "restart" ? "Restart service" : "Run command"}</button></div></dialog>;
}
