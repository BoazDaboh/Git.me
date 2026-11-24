"use client";
import { useState, type FormEvent } from "react";
const key = "gitme.hidden-code.v1";
export const hiddenRootKey = "gitme.hidden-root.v1";
async function digest(code: string, salt: string) {
  const bytes = new TextEncoder().encode(`${salt}:${code}`);
  return Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes)), b => b.toString(16).padStart(2,"0")).join("");
}
export default function HiddenAccess({ close, onUnlock }: {close:()=>void;onUnlock:()=>void}) {
  const [setup] = useState(() => !localStorage.getItem(key));
  const [error,setError] = useState("");
  const [busy,setBusy] = useState(false);
  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = new FormData(event.currentTarget); const code=String(form.get("code") || "");
    if (code.length < 4 || code.length > 128) {setError("Use a code of 4 to 128 characters.");return;}
    setBusy(true);setError("");
    try {
      if (setup) {
        if (code !== String(form.get("confirm") || "")) { setError("Codes do not match.");return; }
        const salt=Array.from(crypto.getRandomValues(new Uint8Array(16)), b=>b.toString(16).padStart(2,"0")).join("");
        localStorage.setItem(key,JSON.stringify({salt,hash:await digest(code,salt)}));
      } else {
        const saved=JSON.parse(localStorage.getItem(key) || "null");
        if (!saved || typeof saved.salt !== "string" || typeof saved.hash !== "string" || await digest(code,saved.salt) !== saved.hash) { setError("Incorrect code.");return; }
      }
      onUnlock();
    } catch {setError("Could not access the local code in this browser.");} finally {setBusy(false);}
  }
  return <form className="editor-form" onSubmit={submit}><p className="muted">{setup ? "Set a code for the hidden projects section on this browser." : "Enter your hidden projects code."}</p><label>Code<input type="password" name="code" minLength={4} maxLength={128} required autoFocus autoComplete="off"/></label>{setup && <label>Confirm code<input type="password" name="confirm" minLength={4} maxLength={128} required autoComplete="off"/></label>}{error && <p className="project-form-error" role="alert">{error}</p>}<div className="dialog-footer"><button className="button" type="button" onClick={close} disabled={busy}>Cancel</button><button className="button primary" disabled={busy}>{setup ? "Set code" : "Open"}</button></div></form>;
}
