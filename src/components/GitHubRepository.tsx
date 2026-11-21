"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useWorkspace, updateWorkspace, useWorkspaceStorage } from "@/lib/store";
import { githubRequest, repoSuggestion, repoURL, validRepoName, type GitHubActivity, type GitHubLink, type GitHubPage, type GitHubRepo, type GitHubSavedConnection } from "@/lib/github";
import { useGitHub } from "./GitHubConnection";
import Icon from "./Icon";

export default function GitHubRepository({ projectId, serviceId, mode = "activity" }: { projectId: string; serviceId: string; mode?: "connection" | "activity" }) {
  const data = useWorkspace(); const github = useGitHub();
  const project = data.projects.find((item) => item.id === projectId);
  if (!project) return null;
  const profile = project.services?.find((item) => item.id === serviceId);
  if (!profile) return null;
  const link = profile.github;
  const pendingLinks = project.unassignedGitHubLinks || [];
  const sources = [profile.repository, ...pendingLinks.map((item) => repoURL(item.fullName))];
  const suggestions = [...new Set(sources.map(repoSuggestion).filter((value): value is string => !!value))].slice(0, 10);
  const identity = `${github.available}:${github.connections.map((item) => `${item.id}:${item.connected}:${item.checked_at}`).join(",")}`;
  return <RepositoryBody key={`${projectId}:${serviceId}:${mode}:${link?.id || "none"}:${link?.connectionId || "legacy"}:${link?.fullName || ""}:${identity}`} projectId={projectId} serviceId={serviceId} mode={mode} link={link} suggestions={suggestions} pendingLinks={pendingLinks}/>;
}

function RepositoryBody({ projectId, serviceId, mode, link, suggestions, pendingLinks }: { projectId: string; serviceId: string; mode: "connection" | "activity"; link?: GitHubLink; suggestions: string[]; pendingLinks: GitHubLink[] }) {
  const github = useGitHub(); const storage = useWorkspaceStorage();
  const connected = github.available && github.connections.some((item) => item.connected);
  const linkedAccount = github.connections.find((item) => !link?.connectionId || link.connectionId === "legacy" ? item.legacy : item.id === link.connectionId);
  const [accountId, setAccountId] = useState(() => linkedAccount?.connected ? linkedAccount.id : github.connections.find((item) => item.connected)?.id || "");
  const requestVersion = useRef(0);
  const [activity, setActivity] = useState<GitHubActivity | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [picker, setPicker] = useState(false);
  const [page, setPage] = useState<GitHubPage | null>(null);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<GitHubRepo | null>(null);
  const [picking, setPicking] = useState(false);
  const refresh = useRef<(force?: boolean) => Promise<void>>(async () => {});
  const canRead = github.available && !!linkedAccount?.connected && storage.state === "saved";
  const canPick = connected && storage.state === "saved";
  useEffect(() => () => { requestVersion.current++; }, []);
  useEffect(() => {
    if (mode === "connection" || !link || !canRead) return;
    let active = true, busy = false; let controller: AbortController | null = null;
    const check = async (force = false) => {
      if (!active || busy) return; busy = true; controller = new AbortController(); setLoading(true);
      const params = new URLSearchParams({ project_id: projectId, service_id: serviceId, ...(force ? { refresh: "1" } : {}) });
      try { const result = await githubRequest<GitHubActivity>(`activity?${params}`, undefined, controller.signal); if (active) { setActivity(result); setError(""); } }
      catch (error) { if (active) setError(error instanceof Error ? error.message : "Repository activity unavailable."); }
      finally { busy = false; if (active) setLoading(false); }
    };
    refresh.current = check; const poll = () => { if (document.visibilityState === "visible") void check(); };
    const pushed = (event: Event) => { const detail = (event as CustomEvent<{ projectId: string; serviceId: string }>).detail; if (detail?.projectId === projectId && detail.serviceId === serviceId) void check(true); };
    void check(); const timer = setInterval(poll, 60000);
    window.addEventListener("focus", poll); document.addEventListener("visibilitychange", poll);
    window.addEventListener("gitme:git-pushed", pushed);
    return () => { active = false; controller?.abort(); clearInterval(timer); window.removeEventListener("focus", poll); document.removeEventListener("visibilitychange", poll); window.removeEventListener("gitme:git-pushed", pushed); };
  }, [projectId, serviceId, link, canRead, mode]);

  function save(repo?: GitHubRepo) {
    const value = repo ? { id: repo.id, fullName: repo.full_name, connectionId: accountId } : undefined;
    const accountFor = (saved: GitHubLink) => github.connections.find((item) => !saved.connectionId || saved.connectionId === "legacy" ? item.legacy : item.id === saved.connectionId)?.id || saved.connectionId || "legacy";
    updateWorkspace((data) => ({ ...data, projects: data.projects.map((item) => item.id !== projectId ? item : { ...item, services: (item.services ?? []).map((profile) => profile.id === serviceId ? { ...profile, github: value, ...(repo ? { repository: repoURL(repo.full_name) } : {}) } : profile), ...(repo && item.unassignedGitHubLinks ? { unassignedGitHubLinks: item.unassignedGitHubLinks.filter((saved) => saved.id !== repo.id || accountFor(saved) !== accountId) } : {}) }) }));
    setPicker(false);
  }
  async function repositories(number: number, connectionId = accountId) {
    const version = ++requestVersion.current;
    setPicking(true); setError(""); setSelected(null); setPage(null);
    try { const result = await githubRequest<GitHubPage>(`repositories?page=${number}&connection_id=${encodeURIComponent(connectionId)}`); if (version === requestVersion.current) setPage(result); }
    catch (error) { if (version === requestVersion.current) setError(error instanceof Error ? error.message : "Could not list repositories."); }
    finally { if (version === requestVersion.current) setPicking(false); }
  }
  async function lookup(name: string, connectionId = accountId) {
    if (!validRepoName(name)) { setError("Use owner/repository, such as owner/repository."); return; }
    const version = ++requestVersion.current;
    setPicking(true); setError(""); setSelected(null);
    try { const result = await githubRequest<GitHubRepo>(`repository?full_name=${encodeURIComponent(name)}&connection_id=${encodeURIComponent(connectionId)}`); if (version === requestVersion.current) setSelected(result); }
    catch (error) { if (version === requestVersion.current) setError(error instanceof Error ? error.message : "Repository unavailable."); }
    finally { if (version === requestVersion.current) setPicking(false); }
  }
  return <section className="panel github-repository" aria-label="Service GitHub repository"><div className="panel-heading"><h2><Icon name="branch" size={16}/>{mode === "connection" ? "GitHub connection" : "GitHub repository"}</h2><span className="badge"><i/>{mode === "connection" ? "GitHub" : "Read-only"}</span></div><div className="github-repository-body"><div className="github-repository-heading"><div>{link ? <a href={repoURL(link.fullName)} target="_blank" rel="noreferrer" className="github-repo-name">{link.fullName}<Icon name="external" size={13}/></a> : <h3>No repository linked</h3>}<p>{link ? "Linked to this service" : "Choose the matching GitHub repository."}</p>{mode === "activity" && activity && <small>Last checked {new Date(activity.checked_at).toLocaleTimeString("en-GB")}{error ? " · Last successful reading" : ""}</small>}</div><div className="github-actions">{link && <>{mode === "activity" && <button className="button integration-action" disabled={!canRead || loading} onClick={() => { void refresh.current(true); }}><Icon name="refresh" size={15}/>{loading ? "Refreshing..." : "Refresh"}</button>}<button className="button integration-action integration-destructive" disabled={storage.state !== "saved"} onClick={() => { if (window.confirm("Remove this repository link? Your GitHub repository will not change.")) save(); }}><Icon name="close" size={15}/>Unlink</button></>}<button className={picker ? "button integration-action" : "button primary integration-action"} disabled={!canPick} onClick={() => { const next = !picker; setPicker(next); if (next) void repositories(1); else { requestVersion.current++; setPicking(false); } }}><Icon name={picker ? "close" : "link"} size={16}/>{picker ? "Close picker" : link ? "Change repository" : "Link repository"}</button></div></div>
    {!connected && <p className="github-hint"><Link className="button integration-secondary" href="/settings#github-connection"><Icon name="link" size={16}/>Connect GitHub in Settings<Icon name="arrow" size={13}/></Link>{github.error && <span>{github.error}</span>}</p>}{storage.state !== "saved" && <p className="github-hint">Wait for Saved to SQLite before linking or refreshing.</p>}
    {link && <p className="github-linked-account">{linkedAccount?.account ? `Account: @${linkedAccount.account.login}${linkedAccount.connected ? "" : " (disconnected)"}` : "Linked account disconnected. Choose an account with Change repository."}</p>}
    {picker && connected && <div className="github-picker"><SavedRepositoryLinks links={pendingLinks} accounts={github.connections} busy={picking} choose={(saved, id) => { setAccountId(id); setQuery(saved.fullName); void lookup(saved.fullName, id); }}/><label className="github-picker-account">GitHub account<select aria-label="GitHub account" value={accountId} disabled={picking} onChange={(event) => { const id = event.target.value; setAccountId(id); setQuery(""); void repositories(1,id); }}>{github.connections.filter((item) => item.connected).map((item) => <option key={item.id} value={item.id}>@{item.account?.login || item.id}</option>)}</select></label><label>Filter this page or enter owner/repository<input value={query} onChange={(event) => setQuery(event.target.value)} maxLength={201} placeholder="owner/repository"/></label><div className="github-picker-tools"><button className="button" disabled={picking || !validRepoName(query.trim())} onClick={() => { void lookup(query.trim()); }}>Look up repository</button><span>{page ? `Page ${page.page}` : ""}</span></div>{suggestions.length > 0 && <div className="github-suggestions">{suggestions.map((name) => <button className="text-link" key={name} disabled={picking} onClick={() => { setQuery(name); void lookup(name); }}>Suggested: {name}</button>)}</div>}<div className="github-repo-list" aria-label="Available GitHub repositories">{page?.repositories.filter((repo) => repo.full_name.toLowerCase().includes(query.toLowerCase())).map((repo) => <button key={repo.id} className={selected?.id === repo.id ? "selected" : ""} aria-pressed={selected?.id === repo.id} disabled={picking} onClick={() => setSelected(repo)}><strong>{repo.full_name}</strong><span>{repo.private ? "Private" : "Public"}{repo.archived ? " · Archived" : ""}</span></button>)}{picking && <p>Reading GitHub...</p>}{page && !picking && page.repositories.filter((repo) => repo.full_name.toLowerCase().includes(query.toLowerCase())).length === 0 && <p>No match on this page. Use owner/repository to look it up directly.</p>}</div><div className="github-pagination"><button className="text-link" disabled={picking || !page || page.page <= 1} onClick={() => { void repositories((page?.page || 1) - 1); }}>Previous page</button><button className="text-link" disabled={picking || !page?.next} onClick={() => { void repositories((page?.page || 1) + 1); }}>Next page</button></div>{selected && <div className="github-selected"><div><strong>{selected.full_name}</strong><p>{selected.private ? "Private" : "Public"} · Default branch: {selected.default_branch || "Not set"}</p></div><button className="button primary integration-action" disabled={picking || storage.state !== "saved"} onClick={() => save(selected)}><Icon name="link" size={16}/>Link selected repository</button></div>}</div>}
    {error && <p className="github-error" role="alert">{error}</p>}{mode === "activity" && loading && !activity && <p className="github-hint" role="status">Reading repository activity...</p>}
    {mode === "activity" && activity && <><div className="github-repo-facts"><span>{activity.repository.private ? "Private" : "Public"}</span><span>Default branch: {activity.repository.default_branch || "Not set"}</span>{activity.repository.archived && <span>Archived</span>}</div>{activity.warnings.length > 0 && <ul className="github-warnings">{activity.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>}<div className="github-activity-grid"><section><h3>Recent commits</h3>{activity.commits.map((commit) => <a className="github-activity-row" key={commit.sha} href={`${repoURL(activity.repository.full_name)}/commit/${encodeURIComponent(commit.sha)}`} target="_blank" rel="noreferrer"><strong>{commit.title || "Commit"}</strong><small>{commit.sha.slice(0, 7)} · {commit.author}{commit.date && ` · ${new Date(commit.date).toLocaleDateString("en-GB")}`}</small></a>)}{activity.commits.length === 0 && <p>{activity.warnings.some((item) => item.startsWith("Commits:")) ? "Commit activity unavailable." : "No commits returned."}</p>}</section><section><h3>Recent open pull requests</h3>{activity.pulls.map((pull) => <a className="github-activity-row" key={pull.number} href={`${repoURL(activity.repository.full_name)}/pull/${pull.number}`} target="_blank" rel="noreferrer"><strong>#{pull.number} {pull.title}</strong><small>{pull.user.login}{pull.draft ? " · Draft" : ""}</small></a>)}{activity.pulls.length === 0 && <p>{activity.warnings.some((item) => item.startsWith("Pull requests:")) ? "Pull requests unavailable." : "No open pull requests returned."}</p>}</section></div><div className="github-branches"><h3>Branches</h3><div className="tags">{activity.branches.map((branch) => <span title={branch.protected ? "Protected branch" : "Branch"} key={branch.name}>{branch.name}</span>)}</div>{activity.branches.length === 0 && <p>{activity.warnings.some((item) => item.startsWith("Branches:")) ? "Branches unavailable." : "No branches returned."}</p>}<a className="text-link" href={`${repoURL(activity.repository.full_name)}/branches`} target="_blank" rel="noreferrer">View all on GitHub<Icon name="external" size={13}/></a></div></>}
    </div></section>;
}




function SavedRepositoryLinks({ links, accounts, busy, choose }: { links: GitHubLink[]; accounts: GitHubSavedConnection[]; busy: boolean; choose: (link: GitHubLink, accountId: string) => void }) {
  if (!links.length) return null;
  return <div className="github-suggestions" aria-label="Saved repository links">{links.map((link) => {
    const account = accounts.find((item) => !link.connectionId || link.connectionId === "legacy" ? item.legacy : item.id === link.connectionId);
    return <button key={`${link.connectionId || "legacy"}:${link.id}:${link.fullName.toLowerCase()}`} className="button integration-secondary" disabled={busy || !account?.connected} title={account?.connected ? "Use this saved repository and its original account" : "Reconnect the original GitHub account in Settings"} onClick={() => { if (account?.connected) choose(link,account.id); }}><Icon name="link" size={14}/>Saved: {link.fullName} · {account?.account?.login ? `@${account.account.login}` : "Account unavailable"}</button>;
  })}</div>;
}

