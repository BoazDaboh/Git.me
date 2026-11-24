"use client";

import Link from "next/link";
import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { companionPollMs, companionTimeoutMs, companionUrl, fetchCompanionHealth, initialConnection, type CompanionConnection } from "@/lib/companion";
import Icon from "./Icon";
import { watchConnectionRefresh } from "@/lib/connection-refresh";

const ConnectionContext = createContext<{ connection: CompanionConnection; refresh: () => void } | null>(null);

export function CompanionProvider({ children }: { children: ReactNode }) {
  const [connection, setConnection] = useState(initialConnection);
  const checkRef = useRef<(() => Promise<void>) | null>(null);
  useEffect(() => {
    let active = true, busy = false, pending = false;
    let previous: CompanionConnection | null = null;
    let controller: AbortController | null = null;
    const check = async () => {
      if (!active) return;
      if (busy) { pending = true; return; }
      busy = true; controller = new AbortController();
      const timeout = window.setTimeout(() => controller?.abort(), companionTimeoutMs);
      try {
        const result = await fetchCompanionHealth(controller.signal);
        if (active) {
          if (result.state === "offline" || result.state === "mismatch") { try { sessionStorage.removeItem("gitme.hidden-until.v1"); } catch {} window.dispatchEvent(new Event("gitme:hidden-lock")); }
          setConnection(current => ({ ...result, lastConnectedAt: result.state === "connected" ? result.checkedAt ?? undefined : current.lastConnectedAt }));
          if (result.state === "connected" && previous && (previous.state !== "connected" || previous.startedAt !== result.startedAt)) window.dispatchEvent(new Event("gitme:companion-ready"));
          previous = result;
        }
      } finally { clearTimeout(timeout); busy = false; if (active && pending) { pending = false; watcher.wake(); } }
    };
    checkRef.current = check;
    const watcher = watchConnectionRefresh(check, companionPollMs);
    void check();
    return () => { active = false; watcher.stop(); controller?.abort(); checkRef.current = null; };
  }, []);
  const refresh = useCallback(() => {
    setConnection((value) => ({ ...value, checking: true }));
    void checkRef.current?.();
  }, []);
  return <ConnectionContext.Provider value={{ connection, refresh }}>{children}</ConnectionContext.Provider>;
}

export function useCompanion() {
  const context = useContext(ConnectionContext);
  if (!context) throw new Error("CompanionProvider is required.");
  return context;
}

function statusLabel(connection: CompanionConnection) {
  if (connection.checking) return "Checking";
  return connection.state === "connected" ? "Connected" : connection.state === "mismatch" ? "Unexpected service" : "Offline";
}

export function CompanionBadge() {
  const { connection } = useCompanion();
  return <span className={`badge companion-badge ${connection.state}`} role="status" aria-live="polite"><i/>{statusLabel(connection)}</span>;
}

export function CompanionChip() {
  const { connection } = useCompanion();
  const label = statusLabel(connection);
  return <Link href="/settings#local-companion" className={`companion-chip ${connection.state} ${connection.checking ? "checking" : ""}`} aria-label={`Companion ${label.toLowerCase()}`} title={`Companion: ${label}. ${connection.message}`}><span className="companion-dot"/><span className="companion-chip-label">Companion {label.toLowerCase()}</span></Link>;
}

export function CompanionPanel({ notify }: { notify: (message: string) => void }) {
  const { connection, refresh } = useCompanion();
  const checked = connection.checkedAt ? new Date(connection.checkedAt).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", second: "2-digit" }) : "Not checked yet";
  const startup = "Start GitMe.cmd";
  return <section id="local-companion" className="panel companion-settings"><div className="panel-heading"><h2><Icon name="terminal" size={17}/>Local companion</h2><CompanionBadge/></div><div className="companion-panel-body"><div className="companion-check-row"><div><h3>{connection.state === "connected" ? "Connection established" : connection.state === "mismatch" ? "Check the service on this port" : "Connect your local companion"}</h3><p role="status">{connection.message}</p></div><button className="button" onClick={refresh} disabled={connection.checking}><Icon name="monitor" size={15}/>{connection.checking ? "Checking..." : "Check connection"}</button></div><div className="companion-readings"><div><small>Endpoint</small><a href={companionUrl} target="_blank" rel="noreferrer">127.0.0.1:47821/health<Icon name="external" size={12}/></a></div><div><small>Version</small><strong>{connection.version ?? "Not available"}</strong></div><div><small>Last checked</small><strong>{checked}</strong></div><div><small>Response time</small><strong>{connection.latencyMs === null ? "Not available" : `${connection.latencyMs} ms`}</strong></div></div><div className="companion-startup"><div><h3>Start Git/Me</h3><button className="text-link" onClick={async () => { try { await navigator.clipboard.writeText(startup); notify("Launcher name copied."); } catch { notify("Clipboard unavailable. Select and copy the commands manually."); } }}><Icon name="copy" size={13}/>Copy launcher name</button></div><pre>{startup}</pre><p>SQLite storage, project scans and reviewed development commands are available.</p></div></div></section>;
}
