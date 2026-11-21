"use client";

import { useState } from "react";
import Icon from "./Icon";

export default function GitCommand({ title, command, description, empty }: { title: string; command: string; description: string; empty?: string }) {
  const [feedback, setFeedback] = useState<{ command: string; message: string } | null>(null);
  async function copy() {
    try { await navigator.clipboard.writeText(command); setFeedback({ command, message: "Copied" }); }
    catch { setFeedback({ command, message: "Select the command and copy it manually." }); }
  }
  return <section className="git-command" aria-label={title}><div className="git-command-heading"><h4>{title}</h4>{command && <button className="button integration-secondary" type="button" aria-label={`Copy ${title} command`} onClick={() => { void copy(); }}><Icon name="copy" size={14}/>Copy</button>}</div>
    {command ? <><pre tabIndex={0}><code>{command}</code></pre><p>{description}</p></> : <p>{empty}</p>}
    {feedback?.command === command && <p className="git-command-feedback" role="status">{feedback.message}</p>}
  </section>;
}
