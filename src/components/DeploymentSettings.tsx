"use client";

import { useId, useState, type FormEvent } from "react";
import { updateWorkspace, useWorkspaceStorage } from "@/lib/store";
import { deploymentMethods, validateService, type ServiceProfile } from "@/lib/services";
import { projectURL } from "@/lib/vercel";
import Icon from "./Icon";

const hostedConfig = (profile: ServiceProfile) => JSON.stringify(profile.deployment);

export default function DeploymentSettings({ projectId, profile, notify }: { projectId: string; profile: ServiceProfile; notify: (message:string) => void }) {
  const storage = useWorkspaceStorage();
  const providersId = useId();
  const [editing,setEditing] = useState(false);
  const [deployment,setDeployment] = useState(profile.deployment);
  const [baseline,setBaseline] = useState(hostedConfig(profile));
  const [error,setError] = useState("");
  function edit() {setDeployment(profile.deployment);setBaseline(hostedConfig(profile));setError("");setEditing(true);}
  function save(event:FormEvent) {
    event.preventDefault();if (storage.state !== "saved") return;
    try {
      const saved = updateWorkspace((data) => {
        const current = data.projects.find((project) => project.id === projectId)?.services?.find((service) => service.id === profile.id);
        if (!current || hostedConfig(current) !== baseline) throw new Error("Deployment configuration changed. Cancel and reopen it to load the latest settings.");
        const next:ServiceProfile = {...current,deployment:{...deployment,provider:deployment.provider.trim()},updatedAt:Date.now()};
        const invalid = validateService(next,next.port?.toString() || "",{local:next.environments.local.variables.join(","),preview:next.environments.preview.variables.join(","),production:next.environments.production.variables.join(",")});
        if (invalid) throw new Error(invalid.message);
        return {...data,projects:data.projects.map((project) => project.id !== projectId ? project : {...project,services:project.services?.map((service) => service.id === profile.id ? next : service)})};
      });
      if (!saved) {setError("Configuration changed for this session only. Browser storage is unavailable.");return;}
      setEditing(false);notify("Deployment reference saved.");
    } catch(error) {setError(error instanceof Error ? error.message : "Could not save deployment configuration.");}
  }
  return <section className="panel deploy-settings" aria-label="Deployment configuration"><div className="panel-heading"><h2>Deployment configuration</h2><div className="github-actions"><span className="badge">Reference only</span>{!editing && <button className="button integration-action" disabled={storage.state !== "saved"} onClick={edit}><Icon name="edit" size={14}/>Edit configuration</button>}</div></div><div className="deploy-settings-body"><p className="git-help">These records do not update your host. Vercel builds with the settings and environment values saved on Vercel.</p>{profile.vercel && <a className="button integration-secondary" target="_blank" rel="noreferrer" href={`${projectURL(profile.vercel.scopeSlug,profile.vercel.projectName)}/settings`}>Manage settings on Vercel<Icon name="external" size={13}/></a>}
    {editing ? <form onSubmit={save}><div className="form-row"><label>Provider<input list={providersId} value={deployment.provider} maxLength={80} placeholder="Vercel, Render, self-hosted..." onChange={(event) => setDeployment({...deployment,provider:event.target.value})}/></label><label>Deployment method<select value={deployment.method} onChange={(event) => setDeployment({...deployment,method:event.target.value as ServiceProfile["deployment"]["method"]})}>{deploymentMethods.map((method) => <option key={method.value} value={method.value}>{method.label}</option>)}</select></label></div><datalist id={providersId}>{["Vercel","Render","AWS","Cloudflare","Netlify","Railway","Fly.io","Self-hosted"].map((name) => <option key={name} value={name}/>)}</datalist><label>Build command<input value={deployment.build} maxLength={1000} onChange={(event) => setDeployment({...deployment,build:event.target.value})}/></label><label>Start command<input value={deployment.start} maxLength={1000} onChange={(event) => setDeployment({...deployment,start:event.target.value})}/></label>{deployment.method === "container" && <div className="form-row"><label>Dockerfile<input value={deployment.dockerfile} maxLength={500} onChange={(event) => setDeployment({...deployment,dockerfile:event.target.value})}/></label><label>Image<input value={deployment.image} maxLength={300} onChange={(event) => setDeployment({...deployment,image:event.target.value})}/></label></div>}{error && <p className="github-error" role="alert">{error}</p>}<div className="github-actions"><button className="button primary integration-action" disabled={storage.state !== "saved"} type="submit">Save configuration</button><button className="button integration-action" type="button" onClick={() => setEditing(false)}>Cancel</button></div></form> : <><dl>{[["Provider",profile.deployment.provider || (profile.vercel ? "Vercel (linked)" : "Not set")],["Method",deploymentMethods.find((method) => method.value === profile.deployment.method)?.label || "Not set"],["Build",profile.deployment.build || "Not recorded"],["Start",profile.deployment.start || "Not recorded"],...(profile.deployment.method === "container" ? [["Dockerfile",profile.deployment.dockerfile || "Not set"],["Image",profile.deployment.image || "Not set"]] : [])].map(([label,value]) => <div key={label}><dt>{label}</dt><dd><code>{value}</code></dd></div>)}</dl></>}
  </div></section>;
}
