"use client";

import { useEffect, useRef, useState } from "react";
import type { ApiPostPlan } from "@/lib/api-tester";
import Icon from "./Icon";

export default function ApiPostReview({plan,body,busy,close,confirm,stop}:{plan:ApiPostPlan;body:string;busy:boolean;close:()=>void;confirm:()=>void;stop:()=>void}){
  const dialog=useRef<HTMLDialogElement>(null);
  const [expired,setExpired]=useState(false);
  useEffect(()=>{const node=dialog.current;node?.showModal();const timer=setTimeout(()=>setExpired(true),Math.max(0,new Date(plan.expires_at).getTime()-Date.now()));return()=>{clearTimeout(timer);node?.close();};},[plan]);
  let formatted=body;try{formatted=JSON.stringify(JSON.parse(body),null,2);}catch{}
  return <dialog ref={dialog} className="dialog git-review api-post-review" aria-labelledby="api-post-review-title" onCancel={event=>{event.preventDefault();if(!busy)close();}}><div className="dialog-heading"><h2 id="api-post-review-title">Review POST request</h2><button className="icon-button" aria-label="Close POST review" disabled={busy} onClick={close}><Icon name="close"/></button></div><div className="git-review-body"><dl><dt>Method</dt><dd>POST</dd><dt>Destination</dt><dd><code>{plan.url}</code></dd><dt>Content type</dt><dd>application/json</dd><dt>JSON size</dt><dd>{plan.body_bytes.toLocaleString()} B</dd></dl><p className="api-post-warning">This POST may create or change data at the destination. A stopped or timed-out request may still have been processed. Git/Me will not retry it automatically.</p><h3>JSON body</h3><pre className="api-response-code" tabIndex={0}>{formatted}</pre>{plan.header_names.length>0&&<p className="api-help">Headers: {plan.header_names.join(", ")}. Header values are hidden.</p>}{expired&&<p className="runtime-error" role="alert">Review expired. Close and review again.</p>}{busy&&<p className="api-notice" role="status">Sending the reviewed POST...</p>}</div><div className="dialog-footer"><button className="button integration-secondary" onClick={busy?stop:close}>{busy?"Stop waiting":"Cancel"}</button><button className="button primary integration-action" disabled={busy||expired} onClick={confirm}><Icon name="send" size={14}/>{busy?"Sending...":"Confirm POST"}</button></div></dialog>;
}
