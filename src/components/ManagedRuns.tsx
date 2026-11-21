"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useWorkspace } from "@/lib/store";
import { processRequest, running, type Run } from "@/lib/processes";
import { projectHref } from "@/lib/workspace";
import Icon from "./Icon";

export default function ManagedRuns({ projectId }: { projectId: string }) {
  const data = useWorkspace();
  const [runs, setRuns] = useState<Run[]>([]);
  const [error, setError] = useState("");
  const [available, setAvailable] = useState(false);
  const [busy, setBusy] = useState(false);
  const refresh = useRef<() => Promise<void>>(async () => {});
  useEffect(() => {
    let mounted = true, checking = false, hasActiveRun = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let controller: AbortController | null = null;
    const check = async () => {
      if (checking || !mounted) return;
      checking = true; controller = new AbortController();
      try { const list = await processRequest<Run[]>("/services/runs", undefined, controller.signal); if (mounted) { hasActiveRun = list.some(running); setRuns(list); setAvailable(true); setError(""); } }
      catch (error) { if (mounted) { setAvailable(false); setError(error instanceof Error ? error.message : "Process list unavailable."); } }
      finally { checking = false; if (mounted) { clearTimeout(timer); timer = setTimeout(() => { if (document.visibilityState === "visible") void check(); else { clearTimeout(timer); timer = setTimeout(poll, 30000); } }, hasActiveRun ? 2000 : 30000); } }
    };
    refresh.current = check;
    const poll = () => { if (document.visibilityState === "visible") void check(); };
    void check();
    window.addEventListener("focus", poll); document.addEventListener("visibilitychange", poll);
    return () => { mounted = false; controller?.abort(); clearTimeout(timer); window.removeEventListener("focus", poll); document.removeEventListener("visibilitychange", poll); };
  }, []);
  const visible = runs.filter((run) => !projectId || run.project_id === projectId).sort((a, b) => Date.parse(b.started_at) - Date.parse(a.started_at));
  return <section className="panel managed-runs"><div className="panel-heading"><h2>Managed local services</h2><button className="text-link" onClick={() => { void refresh.current(); }}>Refresh</button></div><div className="managed-runs-body"><p>Servers started in terminals are not listed here. Start services from their project profiles.</p>{error && <p className="runtime-error" role="alert">{error}</p>}{visible.length === 0 && <p className="runtime-scope">{available ? "No managed launches for this selection." : "Waiting for process status."}</p>}{visible.map((run) => {
    const project = data.projects.find((item) => item.id === run.project_id);
    const exists = project?.services?.some((profile) => profile.id === run.service_id);
    return <article className="managed-run" key={run.run_id}><div><strong>{run.name}</strong><code>{run.folder}</code><code>{run.command}</code><small>{project?.name || "Project removed"} · {available ? run.state.replace("_", " ") : "Last known status"}{run.exit_code !== null ? ` · Exit ${run.exit_code}` : ""}</small>{!exists && <small>Profile is no longer in the workspace. Owned processes can still be stopped here.</small>}</div>{project && exists && <Link className="text-link" href={projectHref(project.id)}>Open project<Icon name="arrow" size={13}/></Link>}<button className="button" disabled={busy || !available || !running(run) || run.state === "stopping"} onClick={async () => {
      setBusy(true); setError("");
      try { await processRequest<Run>("/services/stop", { project_id: run.project_id, service_id: run.service_id, run_id: run.run_id }); await refresh.current(); }
      catch (error) { setError(error instanceof Error ? error.message : "Could not stop the owned process."); }
      finally { setBusy(false); }
    }}>Stop</button></article>;
  })}</div></section>;
}
