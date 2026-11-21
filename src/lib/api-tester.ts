import type { ServiceProfile } from "./services";

export type SavedApiRequest = {id:string;name:string;method:"GET"|"POST";url:string;serviceId:string;updatedAt:number;expectedStatus?:number};
export type ApiHeader = {name:string;value:string};
export type ApiResponse = {status:number;duration_ms:number;bytes:number;body:string;headers:ApiHeader[];truncated:boolean;binary:boolean;message:string;delivery?:"response"|"not-sent"|"unconfirmed"};
export type ApiPostPlan={token:string;url:string;body_bytes:number;header_names:string[];expires_at:string};
export type SavedGetResult={request_id:string;service_id:string;name:string;url:string;expected_status:number;actual_status:number;passed:boolean;duration_ms:number;message:string;checked_at:string};
export const validExpectedStatus=(value:unknown):value is number=>typeof value==="number"&&Number.isInteger(value)&&value>=200&&value<=599;
export const checkSignature=(request:Pick<SavedApiRequest,"id"|"url"|"method"|"serviceId"|"expectedStatus">)=>JSON.stringify([request.id,request.method,request.url,request.serviceId,request.expectedStatus??200]);

export function requestURL(raw:string) {
  try {const url = new URL(raw);if (!/^https?:$/.test(url.protocol)||url.username||url.password||url.hash||["3015","47821"].includes(url.port)||raw.length>4096||/[\x00\r\n\t]/.test(raw))return null;return url;}catch{return null;}
}
export function savedRequestURL(raw:string) {const url=requestURL(raw);if(!url)return "";url.search="";return url.toString();}
export function validSavedApiRequests(value:unknown):value is SavedApiRequest[] {
  if(!Array.isArray(value)||value.length>100)return false;
  const seen=new Set<string>();return value.every(item=>{
    if(!item||typeof item!=="object"||![6,7].includes(Object.keys(item).length)||!Object.keys(item).every(key=>["id","name","method","url","serviceId","updatedAt","expectedStatus"].includes(key))||(item.expectedStatus!==undefined&&!validExpectedStatus(item.expectedStatus)))return false;
    const url=typeof item.url==="string"?requestURL(item.url):null;
    if(typeof item.id!=="string"||!/^[A-Za-z0-9_-]{1,100}$/.test(item.id)||seen.has(item.id)||typeof item.name!=="string"||!item.name.trim()||item.name.length>80||!["GET","POST"].includes(item.method)||!url||url.search||url.href.endsWith("?")||typeof item.serviceId!=="string"||item.serviceId.length>200||typeof item.updatedAt!=="number"||!Number.isFinite(item.updatedAt)||item.updatedAt<=0)return false;
    seen.add(item.id);return true;
  });
}
export function serviceApiURL(service:ServiceProfile,baseURL?:string) {
  const base=baseURL===undefined?service.environments.local.url.trim()||(service.port?`http://127.0.0.1:${service.port}`:""):baseURL;
  const url=requestURL(base);if(!url)return "";
  if(service.healthPath.startsWith("/")&&!service.healthPath.startsWith("//")){try{return new URL(service.healthPath,url).toString();}catch{}}
  return url.toString();
}
export function parseApiHeaders(text:string):ApiHeader[] {
  const rows=text.split(/\r?\n/).filter(row=>row.trim());if(rows.length>24)throw new Error("Use at most 24 headers.");
  const seen=new Set<string>();return rows.map(row=>{const split=row.indexOf(":");if(split<1)throw new Error("Write each header as Name: value.");const name=row.slice(0,split).trim(),value=row.slice(split+1).trim();if(!/^[A-Za-z0-9-]{1,80}$/.test(name)||seen.has(name.toLowerCase()))throw new Error("Use valid, unique header names.");seen.add(name.toLowerCase());return {name,value};});
}
export async function sendApiRequest(projectId:string,serviceId:string,url:string,headers:ApiHeader[],signal:AbortSignal):Promise<ApiResponse> {
  return apiTestRequest<ApiResponse>("send",{project_id:projectId,service_id:serviceId,url,headers},signal);
}
export async function apiTestRequest<T>(path:"send"|"post-review"|"post-execute"|"post-cancel"|"check",body:unknown,signal?:AbortSignal):Promise<T>{
  const response=await fetch(`http://127.0.0.1:47821/api-test/${path}`,{method:"POST",cache:"no-store",signal,headers:{"Content-Type":"application/json","X-GitMe-Client":"workspace-v1"},body:JSON.stringify(body)});
  if(!response.ok){if(response.status===404||response.status===405)throw new Error("Restart the updated Go companion to use API testing.");throw new Error((await response.text()).trim().slice(0,300)||"Request unavailable.");}
  return response.json();
}
export async function runSavedGet(projectId:string,requestId:string,signal:AbortSignal){
  const controller=new AbortController(),abort=()=>controller.abort();if(signal.aborted)abort();signal.addEventListener("abort",abort,{once:true});const timer=setTimeout(abort,13000);
  try{return await apiTestRequest<SavedGetResult>("check",{project_id:projectId,request_id:requestId},controller.signal);}finally{clearTimeout(timer);signal.removeEventListener("abort",abort);}
}
