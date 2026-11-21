"use client";

import { useEffect, useRef, useState } from "react";
import type { Project } from "@/lib/workspace";
import { scanProject, type ProjectScanResult } from "@/lib/project-scan";
import { projectChecks } from "@/lib/project-checks";
import { useWorkspaceStorage } from "@/lib/store";
import Icon from "./Icon";

export default function ProjectChecks({project,copy}:{project:Project;copy:(text:string)=>Promise<void>}) {
  const storage=useWorkspaceStorage();
  const ready=storage.state==="saved";
  const [result,setResult]=useState<ProjectScanResult|null>(null);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const once=useRef(false);
  const refresh=useRef<()=>void>(()=>{});
  useEffect(()=>{
    if(!ready||!project.folder)return;
    let active=true;let controller:AbortController|null=null;let running=false,finished=false;
    const scan=async()=>{
      if(running)return;running=true;finished=false;controller=new AbortController();setBusy(true);setResult(null);setError("");
      const timer=setTimeout(()=>controller?.abort(),8000);
      try{const reading=await scanProject(project.id,controller.signal);if(active){setResult(reading);finished=true;}}
      catch(error){if(active){setError(error instanceof TypeError?"Companion unavailable. Showing saved commands.":error instanceof Error?error.message:"Could not detect checks.");finished=true;}}
      finally{clearTimeout(timer);running=false;if(active)setBusy(false);}
    };
    refresh.current=()=>void scan();
    const timer=setTimeout(()=>{if(!once.current){once.current=true;void scan();}},0);
    return()=>{active=false;clearTimeout(timer);controller?.abort();refresh.current=()=>{};if(!finished)once.current=false;};
  },[project.id,project.folder,ready]);
  const groups=projectChecks(project,result);
  return <section className="panel project-checks" aria-label="Project checks"><div className="panel-heading"><div><h2>Project checks</h2><small>{project.name}</small></div><button className="button integration-action" disabled={!ready||busy||!project.folder} onClick={()=>refresh.current()}><Icon name="refresh" size={14}/>{busy?"Detecting...":"Detect again"}</button></div><div className="project-checks-body"><p className="checks-help">Commands are read from local project files. Copy and run them in the shown folder.</p>{busy&&<p className="checks-help" role="status">Detecting stack and check commands...</p>}{!project.folder&&<p className="checks-help">Set the project folder to detect available checks.</p>}{error&&<p className="runtime-error" role="alert">{error}</p>}{groups.map(group=><section className="checks-service" key={group.id} aria-label={`Checks for ${group.name}`}><div className="checks-service-heading"><h3>{group.name}</h3><span className="badge">{group.detected?"Detected":"Saved profile"}</span></div><code className="checks-folder">{group.folder||"Set a service folder"}</code>{group.stack.length>0&&<div className="tags">{group.stack.map((item,index)=><span key={`${item}:${index}`}>{item}</span>)}</div>}{group.checks.map((check,index)=><div className="check-row" key={`${check.command}:${index}`}><span className="quick-icon"><Icon name="terminal"/></span><div><strong>{check.name}</strong><code>{check.command}</code><small>{check.source==="Detected"?"Detected command":"Saved command; review before running"}</small></div><button className="icon-button" onClick={()=>void copy(check.command)} aria-label={`Copy ${check.name} command for ${group.name}`}><Icon name="copy" size={16}/></button></div>)}{!group.checks.length&&<p className="checks-help">No check command found. Add test or build commands in the service&apos;s local setup.</p>}</section>)}{!groups.length&&!busy&&<p className="checks-help">No supported service found. Set a service profile or check the project folder.</p>}{result&&<p className="checks-help">Checked {new Date(result.scanned_at).toLocaleTimeString("en-GB")}. Commands have not been executed.</p>}{!!result?.warnings.length&&<details className="checks-help"><summary>Detection notes ({result.warnings.length})</summary><ul>{result.warnings.map((warning,index)=><li key={index}>{warning}</li>)}</ul></details>}</div></section>;
}
