"use client";
import { updateWorkspace } from "@/lib/store";
import type { Project } from "@/lib/workspace";
import Icon from "./Icon";

export default function ProjectPin({ project }: { project: Project }) {
  return <button type="button" className={`icon-button project-pin ${project.pinned ? "is-pinned" : ""}`} aria-label={`${project.pinned ? "Unpin" : "Pin"} ${project.name}`} title={`${project.pinned ? "Unpin" : "Pin"} ${project.name}`} aria-pressed={!!project.pinned} onClick={() => updateWorkspace(value => ({ ...value, projects: value.projects.map(item => item.id === project.id ? { ...item, pinned: !item.pinned } : item) }))}><Icon name="pin" size={16}/></button>;
}
