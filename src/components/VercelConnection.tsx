"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { vercelRequest, tokenURL, type VercelConnection } from "@/lib/vercel";
import Icon from "./Icon";
import { watchConnectionRefresh } from "@/lib/connection-refresh";

type Context = {
  connections: VercelConnection[]; available: boolean; error: string;
  refresh: (force?: boolean) => Promise<void>;
  changed: (connection: VercelConnection) => void;
  removed: (id: string) => void;
};
const VercelContext = createContext<Context | null>(null);

export function VercelProvider({ children }: { children: ReactNode }) {
  const [connections, setConnections] = useState<VercelConnection[]>([]);
  const [available, setAvailable] = useState(false);
  const [error, setError] = useState("");
  const check = useRef<(force?: boolean) => Promise<void>>(async () => {});
  const changeVersion = useRef(0);
  useEffect(() => {
    let active = true, busy = false, pendingForce = false;
    let controller: AbortController | null = null;
    const refresh = async (force = false) => {
      if (!active) return;
      if (busy) { pendingForce ||= force; return; }
      busy = true; controller = new AbortController();
      const version = changeVersion.current;
      try {
        const result = await vercelRequest<{ connections: VercelConnection[] }>(`connections${force ? "?refresh=1" : ""}`, undefined, controller.signal);
        if (active && version === changeVersion.current) { setConnections(result.connections); setAvailable(true); setError(""); }
      } catch (error) {
        if (active && version === changeVersion.current) { setAvailable(false); setError(error instanceof Error ? error.message : "Vercel connections unavailable."); }
      } finally { busy = false; if (active && pendingForce) { pendingForce = false; watcher.wake(); } }
    };
    check.current = refresh;
    const watcher = watchConnectionRefresh(refresh, 60000, "gitme.vercel.changed");
    void refresh();
    return () => { active = false; watcher.stop(); controller?.abort(); };
  }, []);
  const refresh = useCallback((force = false) => check.current(force), []);
  const announce = useCallback(() => { changeVersion.current++; setAvailable(true); setError(""); try { localStorage.setItem("gitme.vercel.changed", String(Date.now())); } catch {} }, []);
  const changed = useCallback((value: VercelConnection) => { announce(); setConnections((items) => [...items.filter((item) => item.id !== value.id), value]); }, [announce]);
  const removed = useCallback((id: string) => { announce(); setConnections((items) => items.filter((item) => item.id !== id)); }, [announce]);
  return <VercelContext.Provider value={{ connections, available, error, refresh, changed, removed }}>{children}</VercelContext.Provider>;
}
export function useVercel() { const context = useContext(VercelContext); if (!context) throw new Error("VercelProvider is required"); return context; }

export default function VercelConnectionPanel() {
  const { connections, available, error: connectionError, refresh, changed, removed } = useVercel();
  const [token, setToken] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [replaceID, setReplaceID] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [feedback, setFeedback] = useState("");
  const [showToken, setShowToken] = useState(false);
  const submitting = useRef(false);
  const replacing = connections.find((item) => item.id === replaceID);
  function edit(id = "") { setReplaceID(id); setFormOpen(true); setToken(""); setShowToken(false); setError(""); setFeedback(""); }
  function close() { setFormOpen(false); setReplaceID(""); setToken(""); setShowToken(false); setError(""); setFeedback(""); }
  async function connect() {
    if (submitting.current || !token.trim()) return;
    const secret = token.trim();
    if (!/^[A-Za-z0-9_.-]{20,2100}$/.test(secret)) {
      setFeedback(""); setError("Paste the full Vercel access token, not its name or your password."); return;
    }
    submitting.current = true;
    setShowToken(false); setBusy(true); setError(""); setFeedback("Checking Vercel and saving your connection...");
    try {
      const result = await vercelRequest<VercelConnection>("connect", { token: secret, ...(replaceID ? { connection_id: replaceID } : {}) });
      if (!result.connected || !result.username || !result.id) throw new Error("Vercel did not confirm the connection. Please retry.");
      changed(result); close(); setFeedback(`Connected to Vercel as @${result.username}.`);
    } catch (error) { setFeedback(""); setError(error instanceof Error ? error.message : "Could not connect Vercel."); }
    finally { submitting.current = false; setBusy(false); }
  }
  async function disconnect(connection: VercelConnection) {
    if (!window.confirm(`Remove ${connection.username ? `@${connection.username}` : "this Vercel account"}'s saved token from this PC? Project links stay in your workspace.`)) return;
    setBusy(true); setError(""); setFeedback("");
    try {
      await vercelRequest("disconnect", { connection_id: connection.id }); removed(connection.id);
      if (replaceID === connection.id) close();
      setFeedback("Vercel account disconnected from this PC.");
    } catch (error) { setError(error instanceof Error ? error.message : "Disconnect failed."); }
    finally { setBusy(false); }
  }
  return <section id="vercel-connection" className="panel vercel-connection">
    <div className="panel-heading"><h2><Icon name="branch" size={16}/>Vercel accounts</h2><button className="button primary integration-action" type="button" disabled={busy} onClick={() => edit()}><Icon name="plus" size={16}/>Add account</button></div>
    <div className="vercel-connection-body">
      <p className="github-accounts-intro">Deployment status and confirmed deploy requests. Tokens stay in Windows credentials.</p>
      <div className="github-accounts-list">{connections.map((connection) => <div className="github-account" key={connection.id}>
        <div><strong>{connection.username ? `@${connection.username}` : "Saved Vercel account"}</strong><p>{!available ? "Connection unavailable" : connection.connected ? "Connected" : connection.message || "Check connection"}</p>{connection.checked_at && connection.checked_at !== "0001-01-01T00:00:00Z" && <small>Last checked {new Date(connection.checked_at).toLocaleTimeString("en-GB")}</small>}</div>
        <div className="github-actions"><button type="button" className="button integration-action" disabled={busy} onClick={() => edit(connection.id)}><Icon name="settings" size={15}/>Replace token</button><button type="button" className="button integration-action integration-destructive" disabled={busy} onClick={() => { void disconnect(connection); }}><Icon name="close" size={15}/>Disconnect</button></div>
      </div>)}</div>
      {(formOpen || connections.length === 0) && <div className="vercel-token-form">
        <h3>{replacing ? `Replace token for @${replacing.username || "saved account"}` : "Connect a Vercel account"}</h3>
        <div className="vercel-token-guide"><a className="button integration-secondary" href={tokenURL} target="_blank" rel="noreferrer"><Icon name="link" size={16}/>Create Vercel token<Icon name="external" size={13}/></a><details><summary>Required access</summary><p>Choose the account or team scope you need and set an expiry. Git/Me makes read requests only; the token may allow more access.</p></details></div>
        <div className="vercel-token-entry"><label htmlFor="vercel-token">Vercel access token<input id="vercel-token" type={showToken ? "text" : "password"} disabled={busy} value={token} onChange={(event) => { setToken(event.target.value); setError(""); setFeedback(""); }} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); void connect(); } }} autoComplete="off" spellCheck={false} maxLength={2100} required placeholder="Paste your Vercel token" aria-describedby="vercel-token-hint"/></label><button type="button" className="button" disabled={busy} aria-controls="vercel-token" aria-pressed={showToken} aria-label={showToken ? "Hide token" : "Show token"} onClick={() => setShowToken((value) => !value)}>{showToken ? "Hide" : "Show"}</button></div>
        <p id="vercel-token-hint">Use the full token generated in Vercel.</p>
        <div className="github-actions"><button className="button primary integration-action" type="button" disabled={busy || !token.trim()} onClick={() => { void connect(); }}><Icon name="link" size={16}/>{busy ? "Connecting..." : "Test & connect"}</button>{connections.length > 0 && <button type="button" className="button" disabled={busy} onClick={close}>Cancel</button>}</div>
      </div>}
      <div className="github-connect-feedback" aria-live="polite">{feedback && <p role="status">{feedback}</p>}{error && <p className="github-error" role="alert">{error}</p>}</div>
      <div className="vercel-connection-footer"><button className="button integration-action" disabled={busy} onClick={async () => { setBusy(true); await refresh(true); setBusy(false); }}><Icon name="refresh" size={15}/>Check connections</button><span>Credentials are excluded from workspace exports.</span></div>
      {!error && connectionError && <p className="github-error" role="alert">{connectionError}</p>}
    </div>
  </section>;
}


