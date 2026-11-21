"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { githubRequest, tokenURL, type GitHubSavedConnection } from "@/lib/github";
import Icon from "./Icon";
import { watchConnectionRefresh } from "@/lib/connection-refresh";

type Context = {
  connections: GitHubSavedConnection[]; available: boolean; error: string;
  refresh: (force?: boolean) => Promise<void>;
  changed: (connection: GitHubSavedConnection) => void;
  removed: (id: string) => void;
};
const GitHubContext = createContext<Context | null>(null);

export function GitHubProvider({ children }: { children: ReactNode }) {
  const [connections, setConnections] = useState<GitHubSavedConnection[]>([]);
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
        const result = await githubRequest<{ connections: GitHubSavedConnection[] }>(`connections${force ? "?refresh=1" : ""}`, undefined, controller.signal);
        if (active && version === changeVersion.current) { setConnections(result.connections); setAvailable(true); setError(""); }
      } catch (error) {
        if (active && version === changeVersion.current) { setAvailable(false); setError(error instanceof Error ? error.message : "GitHub connections unavailable."); }
      } finally { busy = false; if (active && pendingForce) { pendingForce = false; watcher.wake(); } }
    };
    check.current = refresh;
    const watcher = watchConnectionRefresh(refresh, 60000, "gitme.github.changed");
    void refresh();
    return () => { active = false; watcher.stop(); controller?.abort(); };
  }, []);
  const refresh = useCallback((force = false) => check.current(force), []);
  const announce = useCallback(() => { changeVersion.current++; setAvailable(true); setError(""); try { localStorage.setItem("gitme.github.changed", String(Date.now())); } catch {} }, []);
  const changed = useCallback((value: GitHubSavedConnection) => { announce(); setConnections((items) => [...items.filter((item) => item.id !== value.id), value]); }, [announce]);
  const removed = useCallback((id: string) => { announce(); setConnections((items) => items.filter((item) => item.id !== id)); }, [announce]);
  return <GitHubContext.Provider value={{ connections, available, error, refresh, changed, removed }}>{children}</GitHubContext.Provider>;
}
export function useGitHub() { const context = useContext(GitHubContext); if (!context) throw new Error("GitHubProvider is required"); return context; }

export default function GitHubConnectionPanel() {
  const { connections, available, error: connectionError, refresh, changed, removed } = useGitHub();
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
    if (!/^(github_pat_|ghp_)[A-Za-z0-9_]{20,}$/.test(secret) || secret.length > 2100) {
      setFeedback(""); setError("Paste the full token beginning with github_pat_ or ghp_, not its name or your GitHub password."); return;
    }
    submitting.current = true;
    setShowToken(false); setBusy(true); setError(""); setFeedback("Checking GitHub and saving your connection...");
    try {
      const result = await githubRequest<GitHubSavedConnection>("connect", { token: secret, ...(replaceID ? { connection_id: replaceID } : {}) });
      if (!result.connected || !result.account?.login || !result.id) throw new Error("GitHub did not confirm the connection. Please retry.");
      changed(result); close(); setFeedback(`Connected to GitHub as @${result.account.login}.`);
    } catch (error) { setFeedback(""); setError(error instanceof Error ? error.message : "Could not connect GitHub."); }
    finally { submitting.current = false; setBusy(false); }
  }
  async function disconnect(connection: GitHubSavedConnection) {
    if (!window.confirm(`Remove ${connection.account ? `@${connection.account.login}` : "this GitHub account"}'s saved token from this PC? Repository links stay in your workspace.`)) return;
    setBusy(true); setError(""); setFeedback("");
    try {
      await githubRequest("disconnect", { connection_id: connection.id }); removed(connection.id);
      if (replaceID === connection.id) close();
      setFeedback("GitHub account disconnected from this PC.");
    } catch (error) { setError(error instanceof Error ? error.message : "Disconnect failed."); }
    finally { setBusy(false); }
  }
  return <section id="github-connection" className="panel github-connection">
    <div className="panel-heading"><h2><Icon name="branch" size={16}/>GitHub accounts</h2><button className="button primary integration-action" type="button" disabled={busy} onClick={() => edit()}><Icon name="plus" size={16}/>Add account</button></div>
    <div className="github-connection-body">
      <p className="github-accounts-intro">Read-only repository activity. Tokens stay in Windows credentials.</p>
      <div className="github-accounts-list">{connections.map((connection) => <div className="github-account" key={connection.id}>
        <div><strong>{connection.account ? `@${connection.account.login}` : "Saved GitHub account"}</strong><p>{!available ? "Connection unavailable" : connection.connected ? "Connected" : connection.message || "Check connection"}</p>{connection.checked_at && connection.checked_at !== "0001-01-01T00:00:00Z" && <small>Last checked {new Date(connection.checked_at).toLocaleTimeString("en-GB")}</small>}</div>
        <div className="github-actions"><button type="button" className="button integration-action" disabled={busy} onClick={() => edit(connection.id)}><Icon name="settings" size={15}/>Replace token</button><button type="button" className="button integration-action integration-destructive" disabled={busy} onClick={() => { void disconnect(connection); }}><Icon name="close" size={15}/>Disconnect</button></div>
      </div>)}</div>
      {(formOpen || connections.length === 0) && <div className="github-token-form">
        <h3>{replacing ? `Replace token for @${replacing.account?.login || "saved account"}` : "Connect a GitHub account"}</h3>
        <div className="github-token-guide"><a className="button integration-secondary" href={tokenURL} target="_blank" rel="noreferrer"><Icon name="link" size={16}/>Create GitHub token<Icon name="external" size={13}/></a><details><summary>Required access</summary><p>Choose the repositories you want to view. Set Contents and Pull requests to Read-only. Metadata is included. Choose an expiry.</p></details></div>
        <div className="github-token-entry"><label htmlFor="github-token">Personal access token<input id="github-token" type={showToken ? "text" : "password"} disabled={busy} value={token} onChange={(event) => { setToken(event.target.value); setError(""); setFeedback(""); }} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); void connect(); } }} autoComplete="off" spellCheck={false} maxLength={2100} required placeholder="github_pat_..." aria-describedby="github-token-hint"/></label><button type="button" className="button" disabled={busy} aria-controls="github-token" aria-pressed={showToken} aria-label={showToken ? "Hide token" : "Show token"} onClick={() => setShowToken((value) => !value)}>{showToken ? "Hide" : "Show"}</button></div>
        <p id="github-token-hint">Paste the full token, including github_pat_ or ghp_.</p>
        <div className="github-actions"><button className="button primary integration-action" type="button" disabled={busy || !token.trim()} onClick={() => { void connect(); }}><Icon name="link" size={16}/>{busy ? "Connecting..." : "Test & connect"}</button>{connections.length > 0 && <button type="button" className="button" disabled={busy} onClick={close}>Cancel</button>}</div>
      </div>}
      <div className="github-connect-feedback" aria-live="polite">{feedback && <p role="status">{feedback}</p>}{error && <p className="github-error" role="alert">{error}</p>}</div>
      <div className="github-connection-footer"><button className="button integration-action" disabled={busy} onClick={async () => { setBusy(true); await refresh(true); setBusy(false); }}><Icon name="refresh" size={15}/>Check connections</button><span>Credentials are excluded from workspace exports.</span></div>
      {!error && connectionError && <p className="github-error" role="alert">{connectionError}</p>}
    </div>
  </section>;
}

