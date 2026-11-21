import Link from "next/link";
import { projectHref, type Project } from "@/lib/workspace";
import Icon from "./Icon";

export default function DebugServiceSetup({projects}:{projects:Project[]}) {
  const missing=projects.filter(project=>!project.services?.length);
  if(!missing.length)return null;
  return <section className="panel debug-service-setup" aria-label="Service profile setup"><div className="panel-heading"><h2>Service setup</h2></div><div className="debug-service-setup-body">{missing.map(project=><div key={project.id}><span><strong>{project.name}</strong><small>No service profile</small></span><Link className="button primary integration-action" href={`${projectHref(project.id)}?newService=1`}><Icon name="plus" size={14}/>Create service profile</Link></div>)}</div></section>;
}
