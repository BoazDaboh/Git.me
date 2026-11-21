"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import { updateWorkspace, useWorkspaceStorage } from "@/lib/store";
import type { Project } from "@/lib/workspace";
import type { ServiceProfile } from "@/lib/services";
import { apiTestRequest, parseApiHeaders, requestURL, savedRequestURL, sendApiRequest, serviceApiURL, validExpectedStatus, type ApiPostPlan, type ApiResponse, type SavedApiRequest } from "@/lib/api-tester";
import Icon from "./Icon";
import ApiPostReview from "./ApiPostReview";
import SavedGetChecks from "./SavedGetChecks";
import { deploymentURL, vercelRequest, type VercelActivity } from "@/lib/vercel";

type AddressMode="local"|"production"|"saved";

export default function ApiTester({project}:{project:Project}) {
  const services=project.services || [];
  const preferred=services.find(service=>service.role==="backend") || services[0];
  const [selection,setSelection]=useState({serviceId:preferred?.id || "",requestId:"",version:0,mode:"local" as AddressMode});
  const storage=useWorkspaceStorage();
  const service=services.find(service=>service.id===selection.serviceId);
  const saved=project.apiRequests?.find(request=>request.id===selection.requestId);
  function choose(request?:SavedApiRequest) {
    setSelection(current=>({serviceId:request ? services.some(service=>service.id===request.serviceId) ? request.serviceId : "" : current.serviceId,requestId:request?.id || "",version:current.version+1,mode:request?"saved":current.mode==="saved"?"local":current.mode}));
  }
  function remove(id:string) {
    if(storage.state!=="saved")return;
    updateWorkspace(data=>({...data,projects:data.projects.map(item=>item.id!==project.id?item:{...item,apiRequests:item.apiRequests?.filter(request=>request.id!==id)})}));
    if(selection.requestId===id)choose();
  }
  return <><SavedGetChecks key={project.id} project={project} open={choose}/><div className="api-tester" aria-label="API tester"><section className="panel api-saved"><div className="panel-heading"><h2>Saved requests</h2><button className="button integration-action" onClick={()=>choose()}><Icon name="plus" size={14}/>New</button></div><div className="api-saved-list">{project.apiRequests?.map(request=><div className={selection.requestId===request.id?"selected":""} key={request.id}><button onClick={()=>choose(request)} aria-pressed={selection.requestId===request.id}><strong>{request.name}</strong><small>{request.method} {request.url}</small></button><button className="icon-button" disabled={storage.state!=="saved"} aria-label={`Delete request ${request.name}`} onClick={()=>remove(request.id)}><Icon name="trash" size={14}/></button></div>)}{!project.apiRequests?.length && <p>No saved requests for {project.name}.</p>}</div></section><div className="api-test-main"><section className="panel api-service-picker"><label>Service<select aria-label="API service" value={service?.id || ""} onChange={event=>setSelection(current=>({serviceId:event.target.value,requestId:"",version:current.version+1,mode:"local"}))}><option value="">Custom address</option>{services.map(service=><option value={service.id} key={service.id}>{service.name}</option>)}</select></label><label>Address<select aria-label="API address" value={selection.mode} onChange={event=>setSelection(current=>({...current,mode:event.target.value as AddressMode,version:current.version+1}))}><option value="local">{service ? "Local" : "Custom address"}</option><option value="production" disabled={!service}>Production</option>{selection.mode === "saved" && <option value="saved">Saved URL</option>}</select></label></section><AddressEditor key={`${selection.version}:${service?.id || "custom"}:${JSON.stringify(service?.vercel)}`} projectId={project.id} projectURL={service?.role === "frontend" ? project.url : ""} mode={selection.mode} service={service} saved={saved} onSaved={id=>setSelection(current=>({...current,requestId:id}))}/></div></div></>;
}

function AddressEditor({projectId,projectURL,mode,service,saved,onSaved}:{projectId:string;projectURL:string;mode:AddressMode;service?:ServiceProfile;saved?:SavedApiRequest;onSaved:(id:string)=>void}) {
  const linked=mode==="production"&&!!service?.vercel;
  const [reading,setReading]=useState<{url:string;hint:string}|null>(null);
  useEffect(()=>{
    if(!linked||!service)return;
    let active=true;const controller=new AbortController();
    void vercelRequest<VercelActivity>(`activity?${new URLSearchParams({project_id:projectId,service_id:service.id})}`,undefined,controller.signal).then(data=>{
      if(!active)return;
      if(data.project.id!==service.vercel?.projectId)throw new Error("Deployment link changed. Choose the address again.");
      const base=data.production?.url?deploymentURL(data.production.url):undefined;
      setReading(base?{url:serviceApiURL(service,base),hint:`Vercel production target · ${data.production!.state}. Edit the route to match your API.`}:{url:"",hint:"Vercel has no production URL for this service. Enter its production endpoint manually."});
    }).catch(error=>{if(active)setReading({url:"",hint:`${error instanceof Error?error.message:"Production address unavailable."} Enter the production URL manually or reconnect in Deploy.`});});
    return()=>{active=false;controller.abort();};
  },[linked,projectId,service]);
  if(linked&&!reading)return <section className="panel api-response" aria-label="Production address"><p className="api-response-empty" role="status">Reading production address...</p></section>;
  let initialURL="",hint="";
  if(mode==="saved"){initialURL=saved?.url||"";hint="Saved URL. Edit the endpoint or choose Local / Production.";}
  else if(mode==="local"){initialURL=service?serviceApiURL(service):"";hint=service?"Local service address. Edit the route to match your API.":"Enter a local or deployed API endpoint.";}
  else if(linked){initialURL=reading?.url||"";hint=reading?.hint||"";}
  else{const recorded=service?.environments.production.url || projectURL;initialURL=service&&recorded?serviceApiURL(service,recorded):"";hint=initialURL?"Recorded production address. Edit it if your deployment has changed.":"Link this service's deployment in Deploy or enter its production endpoint manually.";}
  return <RequestEditor projectId={projectId} service={service} saved={saved} onSaved={onSaved} initialURL={initialURL} addressHint={hint}/>;
}

function RequestEditor({projectId,service,saved,onSaved,initialURL,addressHint}:{projectId:string;service?:ServiceProfile;saved?:SavedApiRequest;onSaved:(id:string)=>void;initialURL:string;addressHint:string}) {
  const storage=useWorkspaceStorage();
  const [url,setURL]=useState(initialURL);
  const [name,setName]=useState(saved?.name || "");
  const [savedId,setSavedId]=useState(saved?.id || "");
  const [method,setMethod]=useState<"GET"|"POST">(saved?.method||"GET");
  const [expectedStatus,setExpectedStatus]=useState(String(saved?.expectedStatus??200));
  const [responseExpected,setResponseExpected]=useState<number|null>(null);
  const [jsonBody,setJsonBody]=useState("");
  const [post,setPost]=useState<{plan:ApiPostPlan;body:string}|null>(null);
  const [reviewing,setReviewing]=useState(false);
  const [headers,setHeaders]=useState("");
  const [showHeaders,setShowHeaders]=useState(false);
  const [response,setResponse]=useState<ApiResponse|null>(null);
  const [view,setView]=useState<"Body"|"Headers">("Body");
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const [notice,setNotice]=useState("");
  const request=useRef<AbortController|null>(null);
  const sending=useRef(false);
  const livePost=useRef<ApiPostPlan|null>(null);
  useEffect(()=>()=>{request.current?.abort();if(livePost.current)void apiTestRequest("post-cancel",{project_id:projectId,service_id:service?.id||"",token:livePost.current.token}).catch(()=>{});},[projectId,service?.id]);

  async function send(event:FormEvent) {
    event.preventDefault();if(sending.current||storage.state!=="saved")return;
    if(!requestURL(url.trim())){setError("Enter a valid HTTP or HTTPS URL without embedded credentials or a fragment.");return;}
    const expected=Number(expectedStatus);if(method==="GET"&&!validExpectedStatus(expected)){setError("Expected HTTP status must be a whole number from 200 to 599.");return;}
    let parsed;try{parsed=parseApiHeaders(headers);}catch(error){setError(error instanceof Error?error.message:"Invalid headers.");return;}
    if(method==="POST"){try{JSON.parse(jsonBody);}catch{setError("Enter valid JSON before reviewing this POST.");return;}if(new TextEncoder().encode(jsonBody).length>65536){setError("JSON body must be 64 KiB or smaller.");return;}}
    sending.current=true;setBusy(true);setError("");setNotice("");setResponse(null);setResponseExpected(null);setView("Body");
    setReviewing(method==="POST");
    const controller=new AbortController();request.current=controller;
    const timer=setTimeout(()=>controller.abort("timeout"),13000);
    try{if(method==="POST"){const plan=await apiTestRequest<ApiPostPlan>("post-review",{project_id:projectId,service_id:service?.id||"",url:url.trim(),headers:parsed,body:jsonBody},controller.signal);if(!controller.signal.aborted){livePost.current=plan;setPost({plan,body:jsonBody});}}else{const result=await sendApiRequest(projectId,service?.id || "",url.trim(),parsed,controller.signal);if(!controller.signal.aborted){setResponse(result);setResponseExpected(expected);}}}
    catch(error){if(!controller.signal.aborted)setError(error instanceof TypeError?"Could not reach the Go companion.":error instanceof Error?error.message:"Request unavailable.");}
    finally{clearTimeout(timer);if(request.current===controller){sending.current=false;setReviewing(false);if(!controller.signal.aborted)setBusy(false);else if(controller.signal.reason==="timeout"){setBusy(false);setError("Request timed out. Check that the companion and API are running.");}}}
  }
  function closePost(){if(!livePost.current||sending.current)return;void apiTestRequest("post-cancel",{project_id:projectId,service_id:service?.id||"",token:livePost.current.token}).catch(()=>{});livePost.current=null;setPost(null);}
  async function confirmPost(){
    if(!post||sending.current||storage.state!=="saved")return;
    sending.current=true;setBusy(true);setReviewing(false);setError("");setNotice("");const controller=new AbortController();request.current=controller;const timer=setTimeout(()=>controller.abort("timeout"),13000);
    try{const result=await apiTestRequest<ApiResponse>("post-execute",{project_id:projectId,service_id:service?.id||"",token:post.plan.token},controller.signal);if(!controller.signal.aborted){livePost.current=null;setPost(null);setResponse(result);}}
    catch(error){if(!controller.signal.aborted){livePost.current=null;setPost(null);setError(error instanceof TypeError?"POST confirmation unavailable. The backend may have processed it. Check the target before sending again.":error instanceof Error?error.message:"POST confirmation unavailable. Check the target before sending again.");}}
    finally{clearTimeout(timer);if(request.current===controller){sending.current=false;setBusy(false);if(controller.signal.reason==="timeout"){livePost.current=null;setPost(null);setError("POST timed out. The backend may have processed it. Check the target before sending again.");}}}
  }
  function stop(){const postSending=method==="POST"&&!reviewing;request.current?.abort();request.current=null;sending.current=false;setBusy(false);setReviewing(false);if(postSending){livePost.current=null;setPost(null);}setNotice(postSending?"Stopped waiting. The backend may have processed this POST. Check the target before sending again.":"Request stopped.");}
  function save() {
    const clean=savedRequestURL(url.trim());if(!clean||!name.trim()){setError("Enter a name and a valid request URL before saving.");return;}
    const expected=Number(expectedStatus);if(method==="GET"&&!validExpectedStatus(expected)){setError("Expected HTTP status must be a whole number from 200 to 599.");return;}
    const id=savedId||crypto.randomUUID();let failure="";
    const stored=updateWorkspace(data=>({...data,projects:data.projects.map(project=>{
      if(project.id!==projectId)return project;
      const requests=project.apiRequests || [];
      const exists=requests.some(item=>item.id===id);
      if(!exists&&requests.length>=100){failure="This project already has 100 saved requests.";return project;}
      const value:SavedApiRequest={id,name:name.trim(),method,url:clean,serviceId:service?.id||"",updatedAt:Date.now(),...(method==="GET"?{expectedStatus:expected}:{})};
      return {...project,apiRequests:exists?requests.map(item=>item.id===id?value:item):[...requests,value]};
    })}));
    if(failure){setError(failure);return;}
    if(!stored){setError("Browser storage unavailable. Changes last for this session only.");return;}
    setSavedId(id);onSaved(id);setError("");setNotice("Request saved. Query values, headers and JSON bodies were not saved.");
  }
  async function copy(){try{await navigator.clipboard.writeText(view==="Body"?response?.body || "":response?.headers.map(header=>`${header.name}: ${header.value}`).join("\n") || "");setNotice("Response copied.");}catch{setError("Clipboard unavailable. Select and copy the response.");}}
  let body=response?.body || "";if(body&&!response?.truncated){try{body=JSON.stringify(JSON.parse(body),null,2);}catch{}}
  return <>{post && <ApiPostReview plan={post.plan} body={post.body} busy={busy} close={closePost} confirm={()=>void confirmPost()} stop={stop}/>}<section className="panel api-request" aria-label="Request builder"><div className="panel-heading"><h2>Request builder</h2><span className="badge">{method}</span></div><form onSubmit={event=>void send(event)} className="api-request-form"><label>HTTP method<select aria-label="HTTP method" value={method} disabled={busy} onChange={event=>{setMethod(event.target.value as "GET"|"POST");setJsonBody("");setResponse(null);setError("");setNotice("");}}><option value="GET">GET</option><option value="POST">POST</option></select></label><label>Request name<input aria-label="Request name" maxLength={80} value={name} disabled={busy} placeholder="Community list" onChange={event=>setName(event.target.value)}/></label><label>Request URL<input aria-label="Request URL" type="url" required value={url} disabled={busy} placeholder="Enter the full API endpoint URL" onChange={event=>{setURL(event.target.value);setResponse(null);}}/></label>{addressHint && <p className="api-help">{addressHint}</p>}{method === "GET" && <label>Expected HTTP status<input aria-label="Expected HTTP status" type="number" min={200} max={599} step={1} value={expectedStatus} disabled={busy} onChange={event=>setExpectedStatus(event.target.value)}/></label>}{method === "POST" && <label>JSON body<textarea aria-label="JSON body" rows={7} maxLength={65536} value={jsonBody} disabled={busy} autoComplete="off" spellCheck={false} placeholder={'{\n  "name": "Ikeja"\n}'} onChange={event=>setJsonBody(event.target.value)}/></label>}<div className="api-request-actions"><button className="button primary integration-action" type="submit" disabled={busy||storage.state!=="saved"||!url.trim()}><Icon name="send" size={14}/>{busy ? reviewing ? "Reviewing..." : "Sending..." : method === "POST" ? "Review POST" : "Send GET"}</button>{busy&&<button className="button integration-secondary" type="button" onClick={stop}>Stop request</button>}<button className="button integration-secondary" type="button" disabled={busy||storage.state!=="saved"||!name.trim()||!requestURL(url.trim())} onClick={save}><Icon name="check" size={14}/>Save request</button><button className="text-link" type="button" onClick={()=>setShowHeaders(value=>!value)} aria-expanded={showHeaders}>Headers</button></div>{showHeaders&&<label>Headers<textarea aria-label="Request headers" rows={4} maxLength={16000} value={headers} disabled={busy} placeholder={"Accept: application/json\nAuthorization: Bearer ..."} autoComplete="off" spellCheck={false} onChange={event=>setHeaders(event.target.value)}/></label>}<p className="api-help">Saved requests keep the URL path. Query values, headers, JSON bodies and responses stay in this session. Requests time out after 10 seconds; redirects are not followed.</p>{notice&&<p className="api-notice" role="status">{notice}</p>}{error&&<p className="runtime-error" role="alert">{error}</p>}</form></section><section className="panel api-response" aria-label="API response"><div className="panel-heading"><h2>Response</h2>{response&&<div className="api-response-metrics"><span className="api-status" data-kind={response.status>=200&&response.status<300?"success":"error"}>{response.status?`HTTP ${response.status}`:"No response"}</span>{responseExpected !== null && <span className="api-check-state" data-state={response.status === responseExpected ? "passed" : "failed"}>{response.status === responseExpected ? "Passed" : "Failed"} · Expected HTTP {responseExpected}</span>}<span>{response.duration_ms} ms</span><span>{response.bytes.toLocaleString()} B{response.truncated?" shown":""}</span></div>}</div>{response?<div className="api-response-body"><div className="api-response-toolbar"><div className="filter-tabs" role="group" aria-label="Response view">{(["Body","Headers"] as const).map(tab=><button key={tab} className={view===tab?"selected":""} aria-pressed={view===tab} onClick={()=>setView(tab)}>{tab}</button>)}</div><button className="button integration-secondary" disabled={view==="Body"&&(!response.body||response.binary)} onClick={()=>void copy()}><Icon name="copy" size={13}/>Copy response</button></div>{response.message&&<p className="api-notice" role="status">{response.message}</p>}{response.truncated&&<p className="api-help">Showing the first 1 MiB of the response.</p>}{view==="Body"?<pre className="api-response-code" tabIndex={0}>{response.binary?"Binary response. Body preview is unavailable.":body||"Empty response body."}</pre>:<pre className="api-response-code" tabIndex={0}>{response.headers.map(header=>`${header.name}: ${header.value}`).join("\n")||"No response headers."}</pre>}</div>:<div className="api-response-empty" role="status">{busy?"Waiting for your API...":"Send a request to view its response."}</div>}</section></>;
}



