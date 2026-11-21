import Image from "next/image";
import type { Project } from "@/lib/workspace";

export default function ProjectMark({ project, small = false, large = false }: { project: Pick<Project, "initials" | "color" | "logo">; small?: boolean; large?: boolean }) {
  return <span className={`project-mark ${project.color} ${small ? "small" : ""} ${large ? "large" : ""} ${project.logo ? "has-logo" : ""}`}>
    {project.logo ? <Image className="project-logo-image" src={project.logo} alt="" width={large ? 64 : 40} height={large ? 64 : 40} unoptimized/> : project.initials}
  </span>;
}
