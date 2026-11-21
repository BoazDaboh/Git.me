"use client";
import { useRef, useState } from "react";
import { updateWorkspace, useWorkspaceStorage } from "@/lib/store";
import { githubRequest, type GitHubActivity } from "@/lib/github";
import type { Project } from "@/lib/workspace";
import { useGitHub } from "./GitHubConnection";
import Icon from "./Icon";

export default function PostActivityPicker({ projects, onSelect }: { projects: Project[]; onSelect: (projectId: string, text: string, platform: string) => void }) {
  const github = useGitHub(), storage = useWorkspaceStorage();
  const [extra, setExtra] = useState("");
  const [platform, setPlatform] = useState("LinkedIn");
  const [tone, setTone] = useState("Natural and concise");
  const [selection, setSelection] = useState("");
  const [activity, setActivity] = useState<GitHubActivity | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [loading, setLoading] = useState(false), [error, setError] = useState("");
  const version = useRef(0);
  const services = projects.flatMap(project => (project.services ?? []).filter(service => service.github).map(service => ({ project, service })));
  const selectedService = services.find(item => JSON.stringify([item.project.id, item.service.id]) === selection);
  const link = selectedService?.service.github;
  const account = github.connections.find(item => !link?.connectionId || link.connectionId === "legacy" ? item.legacy : item.id === link.connectionId);
  const canLoad = !!selectedService && github.available && !!account?.connected && storage.state === "saved";
  async function load() {
    if (!selectedService || !canLoad) return;
    const ticket = ++version.current; setLoading(true); setError(""); setActivity(null); setSelected([]);
    try { const params = new URLSearchParams({ project_id: selectedService.project.id, service_id: selectedService.service.id, refresh: "1" }); const result = await githubRequest<GitHubActivity>(`activity?${params}`); if (version.current === ticket) setActivity(result); }
    catch (err) { if (version.current === ticket) setError(err instanceof Error ? err.message : "Could not load activity."); }
    finally { if (version.current === ticket) setLoading(false); }
  }
  return <section className="panel post-activity"><div className="panel-heading"><h2>GitHub activity</h2><span className="muted">Read-only</span></div><div className="post-activity-body">{services.length ? <><div className="post-activity-controls"><label>Project service<select value={selection} onChange={event => { version.current++; setSelection(event.target.value); setActivity(null); setSelected([]); setError(""); setLoading(false); }}><option value="">Choose a repository</option>{services.map(({ project, service }) => <option key={JSON.stringify([project.id, service.id])} value={JSON.stringify([project.id, service.id])}>{project.name} · {service.name} · {service.github!.fullName}</option>)}</select></label><button className="button" disabled={!canLoad || loading} onClick={() => void load()}><Icon name="refresh" size={15}/>{loading ? "Loading..." : "Load commits"}</button></div>{selectedService && !canLoad && <div className="post-connection-status" role="status">{!github.available && <p>GitHub connection status is unavailable. Check that the Go companion is running.</p>}{github.available && !account?.connected && <p>The GitHub account linked to this service is disconnected. Reconnect that same account in Settings.</p>}{storage.state !== "saved" && <p>Workspace: {storage.message}</p>}</div>}{error && <p role="alert" className="github-error">{error}</p>}{activity && <><p className="muted">{activity.repository.full_name} · Checked {new Date(activity.checked_at).toLocaleString()}</p>{activity.warnings.map(warning => <p className="muted" key={warning}>{warning}</p>)}<div className="post-commits">{activity.commits.map(commit => <label key={commit.sha} className="post-commit"><input type="checkbox" checked={selected.includes(commit.sha)} onChange={event => setSelected(current => event.target.checked ? [...current, commit.sha] : current.filter(id => id !== commit.sha))}/><span><strong>{commit.title || "Commit"}</strong><small>{commit.sha.slice(0, 7)} · {commit.author}{commit.date ? ` · ${new Date(commit.date).toLocaleDateString()}` : ""}</small></span></label>)}{!activity.commits.length && <p className="muted">No commits returned.</p>}</div><div className="post-context-form"><label>Extra context<textarea value={extra} maxLength={3000} placeholder="Why did you build this?" onChange={event => setExtra(event.target.value)}/></label><div className="form-row"><label>Platform<select value={platform} onChange={event => setPlatform(event.target.value)}><option>LinkedIn</option><option>X</option><option>Other</option></select></label><label>Tone<select value={tone} onChange={event => setTone(event.target.value)}><option>Natural and concise</option><option>Professional</option><option>Casual</option></select></label></div><button className="button primary" disabled={!selected.length} onClick={() => { const commits = activity.commits.filter(commit => selected.includes(commit.sha)); const project = selectedService!.project; const context = { projectId: project.id, projectName: project.name, description: project.description, repository: activity.repository.full_name, commits: commits.map(commit => ({ sha: commit.sha, title: commit.title, date: commit.date })), extra: extra.trim(), platform, tone, savedAt: Date.now() }; updateWorkspace(value => ({ ...value, postContexts: [context] })); onSelect(project.id, commits.map(commit => `- ${commit.title} (${commit.sha.slice(0, 7)})`).join("\n"), platform); }}>Save context for AI</button><p className="muted">Includes this project’s description and selected commits. Wait for the workspace to save before asking the AI to read it.</p></div></>}</> : <p className="muted">Link a GitHub repository to a project service to see its commits.</p>}</div></section>;
}
