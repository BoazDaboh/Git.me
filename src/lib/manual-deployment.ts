import type { TrackedRelease } from "./deployment-tracking";
import type { VercelLink } from "./vercel";

export type DeploymentPlan = {
  token: string; release_id: string; link: VercelLink; repository: string;
  branch: string; commit: string; target: "preview" | "production"; expires_at: string;
};
export type DeploymentAction = {
  id: string; release: Pick<TrackedRelease,"id"|"project_id"|"service_id"|"branch"|"commit"|"repository"|"target">;
  environment: "preview" | "production"; created_at: string; warning?: string;
  result: { outcome: "submitting"|"accepted"|"rejected"|"unconfirmed"|"not-sent"; deployment_id: string; url: string; state: string; message: string };
  local?: { project_id:string; service_id:string; target:VercelLink; folder:string; digest:string; files:number; bytes:number };
};
export type LocalUploadPlan = {
  token:string; link:VercelLink; target:"preview"|"production"; expires_at:string;
  source:{folder:string;digest:string;bytes:number;files:{path:string;size:number;sha:string}[];excluded:{path:string;reason:string}[];excluded_count:number};
};
