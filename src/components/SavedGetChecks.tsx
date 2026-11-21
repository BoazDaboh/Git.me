"use client";

import { useEffect, useRef, useState } from "react";
import { checkSignature, runSavedGet, type SavedApiRequest, type SavedGetResult } from "@/lib/api-tester";
import type { Project } from "@/lib/workspace";
import { useWorkspaceStorage } from "@/lib/store";
import Icon from "./Icon";

type Row={source:SavedApiRequest;phase:"pending"|"running"|"done"|"cancelled"|"not-run";result?:SavedGetResult;error?:string};
function rowState(row:Row,requests:SavedApiRequest[]) {
  const current=requests.find(request=>request.id===row.source.id);
  const tested=row.result?{...row.source,url:row.result.url,serviceId:row.result.service_id,expectedStatus:row.result.expected_status}:row.source;
  if(!current||checkSignature(current)!==checkSignature(tested))return "changed";
  return row.phase==="done"?(row.result?.passed?"passed":"failed"):row.phase;
}
const labels={pending:"Pending",running:"Running",passed:"Passed",failed:"Failed",cancelled:"Cancelled","not-run":"Not run",changed:"Changed"};

export default function SavedGetChecks({project,open}:{project:Project;open:(request:SavedApiRequest)=>void}) {
  const storage=useWorkspaceStorage();
  const requests=project.apiRequests||[],gets=requests.filter(request=>request.method==="GET");
  const [rows,setRows]=useState<Row[]>([]);
  const [busy,setBusy]=useState(false);
  const controller=useRef<AbortController|null>(null);
  const running=useRef(false);
  useEffect(()=>()=>{controller.current?.abort();},[]);
  async function run(){
    if(running.current||storage.state!=="saved"||!gets.length)return;
    const snapshot=gets.map(request=>({...request}));
    const active=new AbortController();controller.current=active;running.current=true;setBusy(true);setRows(snapshot.map(source=>({source,phase:"pending"})));
    for(const source of snapshot){
      if(active.signal.aborted)break;
      setRows(current=>current.map(row=>row.source.id===source.id?{...row,phase:"running"}:row));
      try{const result=await runSavedGet(project.id,source.id,active.signal);if(!active.signal.aborted)setRows(current=>current.map(row=>row.source.id===source.id?{...row,phase:"done",result}:row));}
      catch(error){if(!active.signal.aborted)setRows(current=>current.map(row=>row.source.id===source.id?{...row,phase:"not-run",error:error instanceof Error&&error.name!=="AbortError"?error.message:"Check stopped or timed out. Run it again when ready."}:row));}
    }
    if(controller.current===active){running.current=false;setBusy(false);}
  }
  function stop(){controller.current?.abort();controller.current=null;running.current=false;setBusy(false);setRows(current=>current.map(row=>row.phase==="pending"||row.phase==="running"?{...row,phase:"cancelled"}:row));}
  const states=rows.map(row=>rowState(row,requests));
  const summary=[`${states.filter(state=>state==="passed").length} passed`,`${states.filter(state=>state==="failed").length} failed`,...(["pending","running","cancelled","not-run","changed"] as const).flatMap(state=>{const count=states.filter(value=>value===state).length;return count?[`${count} ${labels[state].toLowerCase()}`]:[]})].join(" · ");
  return <section className="panel saved-get-checks" aria-label="Saved GET checks"><div className="panel-heading"><div><h2>Saved GET checks</h2><small>{gets.length} GET {gets.length===1?"request":"requests"}{requests.length>gets.length?` · ${requests.length-gets.length} POST excluded`:""}</small></div><div className="api-check-actions">{busy&&<button className="button integration-secondary" onClick={stop}>Stop checks</button>}<button className="button primary integration-action" disabled={busy||storage.state!=="saved"||!gets.length} onClick={()=>void run()}><Icon name="check" size={14}/>{busy?"Running checks...":"Run saved GET checks"}</button></div></div><div className="saved-get-checks-body"><p className="api-help">Uses saved URLs and expected HTTP status codes. Editor headers and query values are not included. Results stay in this session.</p>{!gets.length&&<p className="api-help">Save a GET request to run status checks.</p>}{!!rows.length&&<><p className="api-check-summary" role="status">{summary}</p><div className="api-check-table-wrap"><table className="api-check-table"><thead><tr><th>Request</th><th>Expected</th><th>Actual</th><th>Result</th><th>Time</th><th aria-label="Actions"/></tr></thead><tbody>{rows.map(row=>{const state=rowState(row,requests),current=requests.find(item=>item.id===row.source.id),expected=row.result?.expected_status??row.source.expectedStatus??200;return <tr key={row.source.id}><td><strong>{row.result?.name||row.source.name}</strong><code>{row.result?.url||row.source.url}</code>{state==="changed"?<small>Definition changed or removed. Run again for a current result.</small>:row.error||row.result?.message?<small>{row.error||row.result?.message}</small>:null}{row.result&&<small>{new Date(row.result.checked_at).toLocaleTimeString("en-GB")}</small>}</td><td>HTTP {expected}</td><td>{row.result?(row.result.actual_status?`HTTP ${row.result.actual_status}`:"No response"):""}</td><td><span className="api-check-state" data-state={state}>{labels[state]}</span></td><td>{row.result?`${row.result.duration_ms} ms`:""}</td><td>{current&&<button className="icon-button" aria-label={`Open request ${current.name}`} onClick={()=>open(current)}><Icon name="arrow" size={14}/></button>}</td></tr>;})}</tbody></table></div></>}</div></section>;
}



