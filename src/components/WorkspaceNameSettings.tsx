"use client";

import { updateWorkspace, useWorkspace } from "@/lib/store";

export default function WorkspaceNameSettings({ notify }: { notify: (message: string) => void }) {
  const data = useWorkspace();
  const name = data.workspaceName ?? "";
  return <form key={name} className="default-directory-setting workspace-name-setting" onSubmit={(event) => {
    event.preventDefault();
    const value = String(new FormData(event.currentTarget).get("workspaceName") ?? "").trim();
    const saved = updateWorkspace((current) => ({ ...current, workspaceName: value }));
    notify(saved ? "Workspace name updated." : "Name changed for this session only. Browser storage is unavailable.");
  }}><label>Workspace name<input name="workspaceName" defaultValue={name} placeholder="My workspace" maxLength={60}/></label><button className="button" type="submit">Save name</button></form>;
}
