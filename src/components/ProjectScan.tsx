"use client";

import { useEffect, useRef, useState } from "react";
import { updateWorkspace, useWorkspaceStorage } from "@/lib/store";
import { matchingProfile, scannedProfile, scanProject, type DetectedService, type ProjectScanResult } from "@/lib/project-scan";
import { serviceCommands, serviceRoles, type ServiceProfile } from "@/lib/services";
import type { Project } from "@/lib/workspace";
import Icon from "./Icon";
import ServiceEditor from "./ServiceEditor";

export default function ProjectScan({ project, edit, notify, openProfile }: { project: Project; edit: () => void; notify: (message: string) => void; openProfile: (profile: ServiceProfile) => void }) {
  const storage = useWorkspaceStorage();
  const [scanning, setScanning] = useState(false);
  const [result, setResult] = useState<ProjectScanResult | null>(null);
  const [error, setError] = useState("");
  const [targets, setTargets] = useState<Record<string, string>>({});
  const [replaceCommands, setReplaceCommands] = useState<Record<string, boolean>>({});
  const [addStack, setAddStack] = useState(true);
  const [editor, setEditor] = useState<{ profile: ServiceProfile; existing: boolean; candidate: DetectedService } | null>(null);
  const [saved, setSaved] = useState<Record<string, string>>({});
  const request = useRef<AbortController | null>(null);
  useEffect(() => () => request.current?.abort(), []);
  async function scan() {
    request.current?.abort();
    const controller = new AbortController(); request.current = controller;
    const timeout = setTimeout(() => controller.abort(), 10000);
    setScanning(true); setError(""); setResult(null);
    try {
      const found = await scanProject(project.id, controller.signal);
      if (!controller.signal.aborted) { setResult(found); setTargets({}); setReplaceCommands({}); setSaved({}); }
    } catch (error) {
      if (request.current === controller) setError(controller.signal.aborted ? "Scan stopped or timed out. Retry with a smaller folder." : error instanceof TypeError ? "Could not reach Go. Start the companion and retry." : error instanceof Error ? error.message : "Could not scan. Check that Go is running.");
    } finally { clearTimeout(timeout); if (request.current === controller) setScanning(false); }
  }
  return <section className="panel project-scan" aria-label="Project discovery"><div className="scan-launcher"><div><h2><Icon name="search" size={16}/>Project discovery</h2><code>{project.folder || "Set a local project folder first."}</code>{storage.state !== "saved" && <p>Wait for Saved to SQLite before scanning.</p>}</div><div className="scan-launcher-actions">{project.folder ? <button className="button" onClick={() => { void scan(); }} disabled={scanning || storage.state !== "saved"}><Icon name="search" size={15}/>{scanning ? "Scanning..." : result ? "Scan again" : "Scan project"}</button> : <button className="button" onClick={edit}>Set folder<Icon name="folder" size={15}/></button>}{result && <button className="icon-button" aria-label="Dismiss scan results" onClick={() => setResult(null)}><Icon name="close" size={15}/></button>}</div></div>
    {scanning && <div className="scan-loading" role="status"><span className="scan-spinner"/>Reading project metadata</div>}
    {error && <p className="scan-error" role="alert">{error}</p>}
    {result && <div className="scan-results"><div className="scan-summary"><span>{result.services.length} {result.services.length === 1 ? "service" : "services"} found · {result.directories} folders checked</span><span>{result.git.found ? result.git.detached ? "Git: detached HEAD" : `Git: ${result.git.branch || "linked metadata"}` : "No Git metadata at this root"}</span></div>{result.git.repository && <div className="scan-repository"><Icon name="branch" size={13}/><code>{result.git.repository}</code></div>}
      {result.warnings.length > 0 && <details className="scan-warnings"><summary>Scan notes ({result.warnings.length})</summary><ul>{result.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul></details>}
      {result.services.length === 0 && <p className="scan-empty">No supported service found. Select the folder containing package.json, go.mod, composer.json, Dockerfile, index.php or index.html.</p>}
      <div className="scan-candidates">{result.services.map((candidate) => {
        const matched = matchingProfile(project, candidate, result);
        const targetId = targets[candidate.key] ?? matched?.id ?? "";
        const target = project.services?.find((profile) => profile.id === targetId);
        return <article className="scan-candidate" key={candidate.key}><div className="scan-candidate-heading"><div><h3>{candidate.name}</h3><code>{candidate.relative_folder}</code></div><span className="badge"><i/>{saved[candidate.key] ? "Profile saved" : "Detected"}</span></div><div className="tags">{candidate.stack.map((tag) => <span key={tag}>{tag}</span>)}</div><div className="scan-facts"><span>{serviceRoles.find((role) => role.value === candidate.role)?.label}</span><span>{candidate.runtime}</span>{candidate.package_manager && <span>{candidate.package_manager}</span>}{candidate.git.branch && <span>Branch: {candidate.git.branch}</span>}</div><details className="scan-command-details"><summary>Files and suggested commands</summary><p className="scan-evidence">{candidate.files.join(", ")}</p><dl>{serviceCommands.map(({ value, label }) => <div key={value}><dt>{label}</dt><dd><code>{candidate.commands[value] || "Not detected"}</code></dd></div>)}</dl>{candidate.notes.map((note) => <p className="scan-candidate-note" key={note}>{note}</p>)}{candidate.dockerfile && <p className="scan-candidate-note">Dockerfile found. Container controls come later.</p>}</details><div className="scan-review"><label>Save as<select aria-label={`Save target for ${candidate.name}`} value={targetId} onChange={(event) => setTargets((current) => ({ ...current, [candidate.key]: event.target.value }))}><option value="">New service profile</option>{(project.services ?? []).map((profile) => <option key={profile.id} value={profile.id}>Update: {profile.name}</option>)}</select></label>{target && <label className="scan-checkbox"><input type="checkbox" checked={!!replaceCommands[candidate.key]} onChange={(event) => setReplaceCommands((current) => ({ ...current, [candidate.key]: event.target.checked }))}/>Use detected local commands</label>}<button className="button" disabled={storage.state !== "saved"} onClick={() => setEditor({ profile: scannedProfile(project, candidate, result, target, !!replaceCommands[candidate.key]), existing: !!target, candidate })}>{target ? "Review update" : "Review profile"}<Icon name="arrow" size={14}/></button>{saved[candidate.key] && <button className="text-link" onClick={() => { const profile = project.services?.find((item) => item.id === saved[candidate.key]); if (profile) openProfile(profile); }}>Open profile<Icon name="arrow" size={13}/></button>}</div>{target && <p className="scan-preserve">Saved fields and commands are kept unless changed in review.</p>}</article>;
      })}</div>{result.services.length > 0 && <label className="scan-checkbox scan-stack-option"><input type="checkbox" checked={addStack} onChange={(event) => setAddStack(event.target.checked)}/>Add reviewed technologies to this project&apos;s stack</label>}
      <p className="scan-footer">Review and save profiles individually. Scanning does not install packages or run commands.</p>
    </div>}
    {editor && <ServiceEditor key={editor.profile.id} initial={editor.profile} existing={editor.existing} projectId={project.id} notify={notify} onClose={() => setEditor(null)} onSaved={(profile) => {
      const candidate = editor.candidate;
      setTargets((current) => ({ ...current, [candidate.key]: profile.id })); setSaved((current) => ({ ...current, [candidate.key]: profile.id }));
      if (addStack) updateWorkspace((data) => ({ ...data, projects: data.projects.map((item) => item.id === project.id ? { ...item, stack: [...new Set([...item.stack, ...profile.stack])].slice(0, 20) } : item) }));
      setEditor(null);
    }}/>} 
  </section>;
}
