"use client";

import { useState } from "react";
import { useWorkspace, updateWorkspace } from "@/lib/store";
import type { Task } from "@/lib/workspace";
import ProjectMark from "./ProjectMark";

const statuses: Task["status"][] = ["Planned", "In progress", "Done"];
type BoardItem = Task & { planId?: string; scheduledDate?: string };

export default function TaskBoard({ tasks, notify, projectId, query = "" }: {
  tasks: Task[];
  projectId?: string;
  query?: string;
  onAddTask: (status: Task["status"]) => void;
  notify: (message: string) => void;
}) {
  const data = useWorkspace();
  const [expanded, setExpanded] = useState<string[]>([]);
  const plans = data.notes.filter(note => note.kind === "plan" && note.planStatus !== "cancelled" && (!projectId || note.project === projectId) && `${note.title} ${note.body}`.toLowerCase().includes(query.toLowerCase().trim()));
  const planTasks: BoardItem[] = plans.map(plan => ({ id: `plan:${plan.id}`, planId: plan.id, title: plan.title || "Untitled plan", project: plan.project, priority: "Plan", status: plan.planStatus === "done" ? "Done" : plan.planStatus === "in-progress" ? "In progress" : "Planned", scheduledDate: plan.scheduledDate }));
  const boardTasks: BoardItem[] = [...tasks, ...planTasks];
  const projectIds = [...new Set(boardTasks.map(item => item.project))];
  projectIds.sort((a, b) => Number(!!data.projects.find(project => project.id === b)?.pinned) - Number(!!data.projects.find(project => project.id === a)?.pinned) || data.projects.findIndex(project => project.id === a) - data.projects.findIndex(project => project.id === b));

  function changeStatus(item: BoardItem, status: Task["status"]) {
    const saved = updateWorkspace(value => item.planId ? ({ ...value, notes: value.notes.map(note => note.id === item.planId ? { ...note, planStatus: status === "Done" ? "done" : status === "In progress" ? "in-progress" : "planned", updatedAt: Date.now() } : note) }) : ({ ...value, tasks: value.tasks.map(task => task.id === item.id ? { ...task, status } : task) }));
    if (!saved) notify("Browser storage unavailable. Changes last for this session only.");
  }

  return <div className="workflow-project-rows">{projectIds.map(id => {
    const project = data.projects.find(item => item.id === id);
    const items = boardTasks.filter(item => item.project === id);
    const open = expanded.includes(id);
    return <section className="panel workflow-project-row" key={id}>
      <div className="workflow-project-heading"><div>{project ? <ProjectMark project={project} small/> : <span className="group-project-initials">?</span>}<strong>{project?.name ?? "Unassigned"}</strong><span className="group-task-count">{items.length}</span></div>{!projectId && statuses.some(status => items.filter(item => item.status === status).length > 1) && <button className="text-link" aria-expanded={open} onClick={() => setExpanded(current => open ? current.filter(value => value !== id) : [...current, id])}>{open ? "View less" : "View more"}</button>}</div>
      <div className="workflow-project-columns">{statuses.map(status => {
        const column = items.filter(item => item.status === status);
        return <div className="workflow-status-column" key={status}><h3>{status}<span>{column.length}</span></h3><div className="workflow-status-cards">{(projectId || open ? column : column.slice(0, 1)).map(item => <article className="task-card" key={item.id}><h3>{item.title}</h3><div className="group-task-footer"><span className={item.priority === "High" ? "priority" : "muted"}>{item.priority}{item.scheduledDate ? ` · ${item.scheduledDate}` : ""}</span><select aria-label={`Status for ${item.title}`} value={item.status} onChange={event => changeStatus(item, event.target.value as Task["status"])}>{statuses.map(option => <option key={option}>{option}</option>)}</select></div></article>)}{column.length === 0 && <p className="workflow-column-empty">No items</p>}</div></div>;
      })}</div>
    </section>;
  })}{projectIds.length === 0 && <div className="panel board-empty">No tasks or plans yet.</div>}</div>;
}
