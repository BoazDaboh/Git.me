"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { importWorkspace, loadDatabaseCopy, resetWorkspace, startWorkspaceSync, syncWorkspace, useWorkspace, useWorkspaceStorage } from "@/lib/store";
import { downloadText } from "@/lib/workspace";
import Icon from "./Icon";

export function WorkspacePersistence() {
  useEffect(() => startWorkspaceSync(), []);
  return null;
}

export function StorageLabel() {
  const storage = useWorkspaceStorage();
  return <span className={`storage-label ${storage.state}`} title={storage.message} role="status">{storage.state === "saved" ? "Saved to SQLite" : storage.state === "saving" ? "Pending save" : storage.state === "loading" ? "Connecting storage" : storage.state === "conflict" ? "Review pending edits" : storage.state === "error" ? "Save failed" : "Browser cache only"}</span>;
}

export function StorageNotice() {
  const storage = useWorkspaceStorage();
  const data = useWorkspace();
  const [exported, setExported] = useState(false);
  if (storage.state === "saved" && storage.cacheAvailable) return null;
  if (storage.state === "saving" || storage.state === "loading") return null;
  return <div className={`storage-notice storage-notice-${!storage.cacheAvailable ? "error" : storage.state}`} role="alert"><Icon name="folder" size={16}/><span>{!storage.cacheAvailable ? "Browser cache unavailable. Wait for Saved to SQLite before closing, or export a copy in Settings." : storage.message}</span>{storage.state === "conflict" ? <div className="storage-notice-actions"><button className="button" onClick={() => { downloadText("gitme-pending-workspace.json", JSON.stringify(data, null, 2), "application/json"); setExported(true); }}>Export pending edits</button><button className="button" disabled={!exported} title={!exported ? "Export pending edits first" : undefined} onClick={() => { if (window.confirm("Load the database copy? Pending edits in this window will be replaced. Make sure your export downloaded first.")) void loadDatabaseCopy(); }}>Load database copy</button></div> : <Link href="/settings#workspace-storage">Storage settings<Icon name="arrow" size={13}/></Link>}</div>;
}

export default function WorkspaceStorage({ notify }: { notify: (message: string) => void }) {
  const data = useWorkspace();
  const storage = useWorkspaceStorage();
  const fileRef = useRef<HTMLInputElement>(null);
  const [importing, setImporting] = useState(false);
  return <section id="workspace-storage" className="panel settings-panel workspace-storage"><div className="panel-heading"><h2>Your data</h2><Icon name="folder"/></div><div className="storage-detail"><StorageLabel/>{storage.state !== "saved" && <p>{storage.message}</p>}<dl><dt>Database file</dt><dd>{storage.path || "Start the updated Go companion to see its database path."}</dd><dt>Saved revision</dt><dd>{storage.revision || "Not saved yet"}</dd></dl><div className="storage-actions"><button className="button" onClick={() => { void syncWorkspace(); }} disabled={storage.state === "loading" || storage.state === "saving" || storage.state === "conflict"}>Retry connection</button><button className="button" onClick={() => downloadText("gitme-workspace.json", JSON.stringify(data, null, 2), "application/json")}><Icon name="download" size={15}/>Export workspace</button><button className="button" disabled={importing || storage.state !== "saved"} onClick={() => fileRef.current?.click()}>Import workspace</button></div><input ref={fileRef} type="file" accept=".json,application/json" className="sr-only" aria-label="Import workspace JSON" onChange={async (event) => {
    const file = event.target.files?.[0]; event.target.value = "";
    if (!file) return;
    if (file.size > 20 * 1024 * 1024) { notify("Choose a workspace export smaller than 20 MB."); return; }
    setImporting(true);
    try {
      const value: unknown = JSON.parse(await file.text());
      if (!window.confirm("Replace this workspace with the selected export? Downloading your current copy first is recommended.")) return;
      const cached = importWorkspace(value);
      notify(cached ? "Import queued. Wait for Saved to SQLite." : "Import queued. Browser cache unavailable; wait for Saved to SQLite.");
    } catch (error) { notify(error instanceof Error ? error.message : "Could not import this file."); }
    finally { setImporting(false); }
  }}/>{storage.state === "conflict" && <div className="storage-conflict"><p>Your edits have not replaced the database. Export them before loading the saved database copy.</p><button className="button" onClick={() => { if (window.confirm("Load the database copy and discard pending browser edits? Export them first if needed.")) void loadDatabaseCopy(); }}>Load database copy</button></div>}<p className="storage-help">SQLite stays on this PC. Export copies to another location for backup. Pending offline edits remain in this browser until saved.</p><button className="text-link" onClick={() => {
    try { const backup = localStorage.getItem("gitme.workspace.before-sqlite.v1") ?? localStorage.getItem("gitme.workspace.v1"); if (!backup) { notify("No earlier browser backup found."); return; } downloadText("gitme-before-sqlite.json", backup, "application/json"); }
    catch { notify("Browser backup is unavailable."); }
  }}>Export original browser backup<Icon name="download" size={13}/></button></div><div className="reset-area"><h3>Reset workspace</h3><p>Restore an empty workspace. This also replaces the saved SQLite workspace.</p><button className="button danger" disabled={storage.state !== "saved"} onClick={() => { if (window.confirm("Reset all workspace records to an empty workspace? Your current copy will download first.")) { downloadText("gitme-before-reset.json", JSON.stringify(data, null, 2), "application/json"); resetWorkspace(); notify("Reset queued. Wait for Saved to SQLite."); } }}>Reset workspace</button></div></section>;
}
