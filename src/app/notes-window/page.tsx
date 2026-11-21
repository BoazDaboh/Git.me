"use client";

import { useEffect, useState } from "react";
import Notes from "@/components/Notes";
import WindowHistory from "@/components/WindowHistory";
import ThemeToggle from "@/components/ThemeToggle";
import { StorageNotice } from "@/components/WorkspaceStorage";

export default function NotesWindow() {
  const [query, setQuery] = useState("");
  const [message, setMessage] = useState("");
  useEffect(() => { if (!message) return; const timer = setTimeout(() => setMessage(""), 4500); return () => clearTimeout(timer); }, [message]);
  return <main className="notes-window"><header className="notes-window-header"><div className="notes-window-title"><WindowHistory/><h1>Notes &amp; plans</h1></div><div className="notes-window-tools"><div className="notes-window-search"><input type="search" aria-label="Search notes and plans" placeholder="Search notes and plans" value={query} onChange={event => setQuery(event.target.value)}/></div><ThemeToggle/></div></header><StorageNotice/><Notes collapsibleList query={query} notify={setMessage} clearSearch={() => setQuery("")}/>{message && <div className="notes-window-message" role="status">{message}</div>}</main>;
}
