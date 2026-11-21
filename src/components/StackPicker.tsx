"use client";

import { useId, useState } from "react";
import Icon from "./Icon";

export default function StackPicker({ selected, onChange, query, onQueryChange, catalog }: { selected: string[]; onChange: (values: string[]) => void; query: string; onQueryChange: (value: string) => void; catalog: string[] }) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const id = useId();
  const choices = catalog.filter((item) => !selected.some((value) => value.toLowerCase() === item.toLowerCase()) && item.toLowerCase().includes(query.trim().toLowerCase())).slice(0, 8);
  if (query.trim() && !catalog.some((value) => value.toLowerCase() === query.trim().toLowerCase()) && !selected.some((value) => value.toLowerCase() === query.trim().toLowerCase())) choices.push(query.trim());
  const highlighted = Math.min(active, Math.max(0, choices.length - 1));
  function add(value: string) {
    const clean = value.trim().slice(0, 40);
    if (clean && selected.length < 20 && !selected.some((item) => item.toLowerCase() === clean.toLowerCase())) onChange([...selected, clean]);
    onQueryChange(""); setActive(0);
  }
  return <div className="stack-field"><label id={`${id}-label`} htmlFor={`${id}-input`}>Stack</label><div className="stack-picker" onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false); }}>
    <div className="stack-chips">{selected.map((item) => <span key={item}>{item}<button type="button" aria-label={`Remove ${item}`} onClick={() => onChange(selected.filter((value) => value !== item))}><Icon name="close" size={11}/></button></span>)}</div>
    <input id={`${id}-input`} role="combobox" aria-labelledby={`${id}-label`} aria-autocomplete="list" aria-expanded={open && choices.length > 0} aria-controls={`${id}-list`} aria-activedescendant={open && choices.length ? `${id}-option-${highlighted}` : undefined} value={query} placeholder={selected.length ? "Add another technology..." : "Search or add a technology..."} maxLength={40} autoComplete="off" onFocus={() => setOpen(true)} onChange={(event) => { onQueryChange(event.target.value); setActive(0); setOpen(true); }} onKeyDown={(event) => {
      if (event.key === "ArrowDown") { event.preventDefault(); setOpen(true); setActive((value) => choices.length ? (value + 1) % choices.length : 0); }
      else if (event.key === "ArrowUp") { event.preventDefault(); setOpen(true); setActive((value) => choices.length ? (value - 1 + choices.length) % choices.length : 0); }
      else if (event.key === "Enter") { event.preventDefault(); if (open && choices.length) add(choices[highlighted]); else if (query.trim()) add(query); setOpen(true); }
      else if (event.key === ",") { event.preventDefault(); if (query.trim()) add(catalog.find((item) => item.toLowerCase() === query.trim().toLowerCase()) ?? query); }
      else if (event.key === "Escape" && open) { event.preventDefault(); event.stopPropagation(); setOpen(false); }
      else if (event.key === "Backspace" && !query) onChange(selected.slice(0, -1));
    }}/>
    {open && choices.length > 0 && <div role="listbox" id={`${id}-list`} aria-label="Technology suggestions" className="stack-options">{choices.map((item, index) => <button type="button" role="option" aria-selected={index === highlighted} id={`${id}-option-${index}`} tabIndex={-1} key={item} className={index === highlighted ? "highlighted" : ""} onMouseDown={(event) => event.preventDefault()} onClick={() => { add(item); setOpen(true); }}>{catalog.includes(item) ? item : `Add "${item}"`}<Icon name="plus" size={13}/></button>)}</div>}
  </div></div>;
}
