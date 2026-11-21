"use client";

import { useRef, useState, type FormEvent } from "react";
import { updateWorkspace, useWorkspace } from "@/lib/store";
import type { Note } from "@/lib/workspace";

const dateKey = (date: Date) => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
const duration = (ms: number) => `${Math.floor(ms / 3600000)}h ${Math.floor(ms / 60000) % 60}m`;

export default function ActivityCalendar({ now, focusNow = now }: { now: number; focusNow?: number }) {
  const data = useWorkspace();
  const [month, setMonth] = useState(() => { const d = new Date(); return new Date(d.getFullYear(), d.getMonth(), 1); });
  const [selected, setSelected] = useState("");
  const [existing, setExisting] = useState("");
  const [existingTask, setExistingTask] = useState("");
  const carousel = useRef<HTMLDivElement>(null);
  const today = now ? dateKey(new Date(now)) : "";
  const first = new Date(month.getFullYear(), month.getMonth(), 1);
  const days = new Date(month.getFullYear(), month.getMonth() + 1, 0).getDate();
  const offset = (first.getDay() + 6) % 7;
  const times: Record<string, number> = {};
  const sessions = [...data.sessions, ...(data.focus && focusNow ? [{ ...data.focus, ended: Math.max(data.focus.started, focusNow) }] : [])];
  for (const session of sessions) {
    let cursor = Math.max(session.started, first.getTime());
    const end = Math.min(session.ended, new Date(month.getFullYear(), month.getMonth() + 1, 1).getTime());
    while (cursor < end) {
      const day = new Date(cursor);
      const next = new Date(day.getFullYear(), day.getMonth(), day.getDate() + 1).getTime();
      const segmentEnd = Math.min(end, next);
      const key = dateKey(day);
      times[key] = (times[key] ?? 0) + segmentEnd - cursor;
      cursor = segmentEnd;
    }
  }
  const projectName = (id: string) => data.projects.find(project => project.id === id)?.name ?? (id ? "Project unavailable" : "No project");
  const plans = data.notes.filter(n => n.kind === "plan" && n.scheduledDate);
  const todaysPlans = plans.filter(n => n.scheduledDate === today && (n.planStatus ?? "planned") !== "done" && n.planStatus !== "cancelled");
  const dayPlans = plans.filter(n => n.scheduledDate === selected);
  const dayTasks = data.tasks.filter(task => task.scheduledDate === selected);
  const dayFull = dayPlans.length + dayTasks.length >= 3;
  const maxTime = Math.max(3600000, ...Object.values(times));
  const keyFor = (day: number) => dateKey(new Date(month.getFullYear(), month.getMonth(), day));
  const monthLabel = month.toLocaleDateString("en-GB", { month: "long", year: "numeric" });
  function changeStatus(id: string, planStatus: NonNullable<Note["planStatus"]>) {
    updateWorkspace(v => ({ ...v, notes: v.notes.map(n => n.id === id ? { ...n, planStatus, updatedAt: Date.now() } : n) }));
  }
  function addPlan(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const title = String(new FormData(form).get("title") ?? "").trim();
    if (!title || dayFull) return;
    const plan: Note = { id: crypto.randomUUID(), title, body: "", kind: "plan", project: String(new FormData(form).get("project") ?? ""), pinned: false, updatedAt: Date.now(), scheduledDate: selected, planStatus: "planned" };
    updateWorkspace(v => ({ ...v, notes: [...v.notes, plan] }));
    form.reset();
  }
  function scheduleExisting() {
    if (!existing || dayFull) return;
    updateWorkspace(v => ({ ...v, notes: v.notes.map(n => n.id === existing ? { ...n, scheduledDate: selected, planStatus: "planned", updatedAt: Date.now() } : n) }));
    setExisting("");
  }
  return <div className="activity-section">
    {todaysPlans.length > 0 && <button className="activity-notice" onClick={() => { const d = new Date(); setMonth(new Date(d.getFullYear(), d.getMonth(), 1)); setSelected(today); }}>Today: {todaysPlans.map(p => `${p.title || "Untitled plan"} (${projectName(p.project)})`).join(", ")}</button>}
    <div className="activity-grid"><section className="panel activity-calendar"><div className="activity-heading"><h2>{monthLabel}</h2><div><button aria-label="Previous month" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))}>‹</button><button aria-label="Next month" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))}>›</button></div></div><div className="calendar-grid">{["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map(day => <span className="calendar-weekday" key={day}>{day}</span>)}{Array.from({ length: offset }, (_, i) => <span key={`blank-${i}`}/>)}{Array.from({ length: days }, (_, index) => { const day = index + 1; const key = keyFor(day); const ms = times[key] ?? 0; const scheduled = plans.filter(p => p.scheduledDate === key); const scheduledTasks = data.tasks.filter(task => task.scheduledDate === key); const markers = [...scheduled.map(plan => ({ id: `plan:${plan.id}`, done: plan.planStatus === "done", cancelled: plan.planStatus === "cancelled", title: plan.title || "Plan" })), ...scheduledTasks.map(task => ({ id: `task:${task.id}`, done: task.status === "Done", cancelled: false, title: task.title }))]; const hasDone = markers.some(marker => marker.done); const level = ms ? Math.max(1, Math.ceil(ms / maxTime * 4)) : 0; return <button key={key} className={`calendar-day level-${level} ${key === today ? "today" : ""} ${key === selected ? "selected" : ""}`} aria-pressed={key === selected} onClick={() => setSelected(key)} aria-label={`${key}, ${duration(ms)} recorded, ${scheduled.length} plans, ${scheduledTasks.length} tasks${hasDone ? ", completed items" : ""}`}><span>{day}</span>{markers.length > 0 && <span className="calendar-markers">{markers.slice(0, 3).map(marker => <i key={marker.id} className={marker.done ? "marker-done" : marker.cancelled ? "marker-cancelled" : "marker-pending"} title={`${marker.title}: ${marker.done ? "Done" : marker.cancelled ? "Cancelled" : "Planned"}`}/>)}</span>}</button>; })}</div><div className="activity-legend"><span><i className="legend-active"/> Work time</span><span><i className="legend-plan"/> Planned</span><span><i className="legend-done"/> Done</span></div></section>
    <section className="panel activity-chart"><div className="activity-heading"><h2>Daily activity</h2><span>{monthLabel}</span></div><div className="chart-area"><div className="chart-scale"><span>{duration(maxTime)}</span><span>0h</span></div><div className="chart-bars">{Array.from({ length: days }, (_, index) => { const day = index + 1, ms = times[keyFor(day)] ?? 0; return <button key={day} onClick={() => setSelected(keyFor(day))} aria-label={`${day}: ${duration(ms)}`} title={`${day}: ${duration(ms)}`}><span style={{ height: ms ? `${Math.max(3, ms / maxTime * 100)}%` : "2px" }}/><small>{day}</small></button>; })}</div></div></section></div>
    {selected && <div className="activity-overlay" onClick={() => setSelected("")} onKeyDown={event => { if (event.key === "Escape") setSelected(""); }}><section className="activity-dialog day-plans" role="dialog" aria-modal="true" aria-label={`Plans for ${selected}`} onClick={event => event.stopPropagation()}><div className="activity-heading"><div><h2>{new Date(`${selected}T12:00:00`).toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "long", year: "numeric" })}</h2><span>{duration(times[selected] ?? 0)} recorded</span></div><button aria-label="Close" onClick={() => setSelected("")}>×</button></div><form className="add-day-plan" onSubmit={addPlan}><input name="title" required maxLength={160} placeholder="Add a plan" aria-label="New plan"/><button className="button primary" disabled={dayFull}>Add</button><select name="project" aria-label="Plan project" defaultValue=""><option value="">No project</option>{data.projects.map(project => <option key={project.id} value={project.id}>{project.name}</option>)}</select></form>{data.notes.some(n => n.kind === "plan" && n.scheduledDate !== selected) && <div className="add-day-plan"><select aria-label="Existing plan" value={existing} onChange={e => setExisting(e.target.value)}><option value="">Choose an existing plan</option>{data.notes.filter(n => n.kind === "plan" && n.scheduledDate !== selected).map(n => <option value={n.id} key={n.id}>{n.title || "Untitled plan"} · {projectName(n.project)}{n.scheduledDate ? ` (${n.scheduledDate})` : ""}</option>)}</select><button className="button" disabled={!existing || dayFull} onClick={scheduleExisting}>Schedule</button></div>}{data.tasks.some(task => task.scheduledDate !== selected) && <div className="add-day-plan"><select aria-label="Existing task" value={existingTask} onChange={event => setExistingTask(event.target.value)}><option value="">Choose an existing task</option>{data.tasks.filter(task => task.scheduledDate !== selected).map(task => <option key={task.id} value={task.id}>{task.title} · {projectName(task.project)}{task.scheduledDate ? ` (${task.scheduledDate})` : ""}</option>)}</select><button className="button" disabled={!existingTask || dayFull} onClick={() => { updateWorkspace(value => ({ ...value, tasks: value.tasks.map(task => task.id === existingTask ? { ...task, scheduledDate: selected } : task) })); setExistingTask(""); }}>Schedule</button></div>}{dayFull && <p className="day-limit-note">This date has three scheduled items. Remove one to add another.</p>}<div className="day-plan-controls"><strong>Plans & tasks</strong><div><button aria-label="Previous plans" onClick={() => carousel.current?.scrollBy({ left: -300, behavior: "smooth" })}>‹</button><button aria-label="Next plans" onClick={() => carousel.current?.scrollBy({ left: 300, behavior: "smooth" })}>›</button></div></div><div className="activity-plans plan-carousel" ref={carousel} tabIndex={0} aria-label="Plans for selected day">{dayPlans.length === 0 && dayTasks.length === 0 && <p className="day-plans-empty">No plans or tasks for this day.</p>}{dayTasks.map(task => <article key={`task:${task.id}`}><div><strong>{task.title}</strong><div className="plan-project">{projectName(task.project)}</div><small>Task · {task.status}</small></div><div className="plan-actions"><button onClick={() => updateWorkspace(value => ({ ...value, tasks: value.tasks.map(item => item.id === task.id ? { ...item, scheduledDate: undefined } : item) }))}>Remove</button><button onClick={() => updateWorkspace(value => ({ ...value, tasks: value.tasks.map(item => item.id === task.id ? { ...item, status: item.status === "Done" ? "Planned" : "Done" } : item) }))}>{task.status === "Done" ? "Reopen" : "Done"}</button></div></article>)}{dayPlans.map(plan => <article key={plan.id}><div><strong>{plan.title || "Untitled plan"}</strong><div className="plan-project">{projectName(plan.project)}</div>{plan.body && <p>{plan.body}</p>}<small>{plan.planStatus ?? "planned"}</small></div><div className="plan-actions"><button onClick={() => updateWorkspace(value => ({ ...value, notes: value.notes.map(item => item.id === plan.id ? { ...item, scheduledDate: undefined } : item) }))}>Remove</button>{(plan.planStatus ?? "planned") !== "done" && plan.planStatus !== "cancelled" ? <><button onClick={() => changeStatus(plan.id, "done")}>Done</button><button onClick={() => changeStatus(plan.id, "cancelled")}>Cancel plan</button></> : <button onClick={() => changeStatus(plan.id, "planned")}>Reopen</button>}</div></article>)}</div></section></div>}
  </div>;
}
