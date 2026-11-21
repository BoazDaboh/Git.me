"use client";

import { useWorkspace, updateWorkspace } from "@/lib/store";
import { defaultProjectDirectory } from "@/lib/workspace";

export default function ProjectDirectorySettings({ notify }: { notify: (message: string) => void }) {
  const data = useWorkspace();
  const base = data.projectDirectory || defaultProjectDirectory;
  return <form key={base} className="default-directory-setting" onSubmit={(event) => {
    event.preventDefault(); const directory = String(new FormData(event.currentTarget).get("directory") ?? "").trim(); if (!directory) return;
    const saved = updateWorkspace((value) => ({ ...value, projectDirectory: directory }));
    notify(saved ? "Default project directory updated." : "Directory changed for this session only. Browser storage is unavailable.");
  }}><label>Default project directory<input name="directory" defaultValue={base} placeholder="Choose a folder, for example C:/Projects" maxLength={500} required/></label><p>New project paths start here. Existing paths stay as you set them.</p><button className="button" type="submit">Save directory</button></form>;
}
