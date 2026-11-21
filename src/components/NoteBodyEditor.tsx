"use client";

import { forwardRef, useImperativeHandle, useLayoutEffect, useRef, type KeyboardEvent } from "react";

type Block = { kind: "text" | "check"; start: number; end: number; line: number; text: string; prefix: number; checked: boolean };
export type NoteBodyHandle = { selection: () => { start: number; end: number }; focusRange: (start: number, end?: number) => void };

function parse(body: string): Block[] {
  const lines = body.split("\n");
  const blocks: Block[] = [];
  let offset = 0, runStart = 0, runLine = 0, run: string[] = [], inCode = false;
  function flush() {
    if (!run.length) return;
    const text = run.join("\n");
    blocks.push({ kind: "text", start: runStart, end: runStart + text.length, line: runLine, text, prefix: 0, checked: false }); run = [];
  }
  lines.forEach((line, index) => {
    if (line.startsWith("```")) inCode = !inCode;
    const match = !inCode && line.match(/^[-*] \[([ xX])\](?: (.*))?$/);
    if (match) {
      flush(); const text = match[2] ?? "";
      blocks.push({ kind: "check", start: offset, end: offset + line.length, line: index, text, prefix: line.length - text.length, checked: match[1].toLowerCase() === "x" });
    } else {
      if (!run.length) { runStart = offset; runLine = index; }
      run.push(line);
    }
    offset += line.length + 1;
  });
  flush(); return blocks;
}

function GrowingField({ value, fieldRef, ...props }: React.TextareaHTMLAttributes<HTMLTextAreaElement> & { fieldRef: (node: HTMLTextAreaElement | null) => void }) {
  const ref = useRef<HTMLTextAreaElement | null>(null);
  useLayoutEffect(() => {
    const node = ref.current; if (!node) return;
    let width = node.clientWidth;
    const resize = () => {
      node.style.height = "auto";
      node.style.height = `${Math.max(25, node.scrollHeight)}px`;
    };
    resize();
    const observer = new ResizeObserver(() => {
      if (node.clientWidth === width) return;
      width = node.clientWidth; resize();
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [value]);
  return <textarea {...props} value={value} rows={1} ref={(node) => { ref.current = node; fieldRef(node); }}/>;
}

const NoteBodyEditor = forwardRef<NoteBodyHandle, { body: string; onChange: (body: string) => void; onFormat: (action: "bold" | "italic") => void }>(function NoteBodyEditor({ body, onChange, onFormat }, ref) {
  const blocks = parse(body);
  const fields = useRef(new Map<number, { node: HTMLTextAreaElement; block: Block }>());
  const lastSelection = useRef({ start: 0, end: 0 });
  const container = useRef<HTMLDivElement>(null);

  function selection() {
    for (const { node, block } of fields.current.values()) {
      if (node === document.activeElement) return { start: block.start + block.prefix + node.selectionStart, end: block.start + block.prefix + node.selectionEnd };
    }
    return lastSelection.current;
  }

  function focusRange(start: number, end = start) {
    const items = [...fields.current.values()];
    const field = items.find(({ block }) => start >= block.start && start <= block.end) ?? items[items.length - 1];
    if (!field) return;
    const from = Math.min(field.node.value.length, Math.max(0, start - field.block.start - field.block.prefix));
    const to = Math.min(field.node.value.length, Math.max(from, end - field.block.start - field.block.prefix));
    field.node.focus(); field.node.setSelectionRange(from, to);
    lastSelection.current = { start, end };
  }

  useImperativeHandle(ref, () => ({ selection, focusRange }));

  function change(next: string, caret?: number) {
    if (next.length > 50000) return;
    onChange(next);
    if (caret !== undefined) requestAnimationFrame(() => focusRange(caret));
  }

  function changeCheck(block: Block, text: string, caret: number) {
    const replacement = text.split("\n").map((line, index) => `${index === 0 && block.checked ? "- [x] " : "- [ ] "}${line}`).join("\n");
    const addedPrefixes = (text.slice(0, caret).match(/\n/g) ?? []).length * 6;
    change(body.slice(0, block.start) + replacement + body.slice(block.end), block.start + 6 + caret + addedPrefixes);
  }

  function keyDown(event: KeyboardEvent<HTMLTextAreaElement>, block: Block) {
    if (event.nativeEvent.isComposing) return;
    const node = event.currentTarget;
    const start = node.selectionStart, end = node.selectionEnd;
    if ((event.ctrlKey || event.metaKey) && ["b", "i"].includes(event.key.toLowerCase())) {
      event.preventDefault(); onFormat(event.key.toLowerCase() === "b" ? "bold" : "italic"); return;
    }
    if (block.kind === "check" && event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      if (!node.value.trim()) {
        change(body.slice(0, block.start) + body.slice(block.end), block.start);
      } else {
        const left = node.value.slice(0, start), right = node.value.slice(end);
        const replacement = `${block.checked ? "- [x] " : "- [ ] "}${left}\n- [ ] ${right}`;
        change(body.slice(0, block.start) + replacement + body.slice(block.end), block.start + 6 + left.length + 7);
      }
      return;
    }
    if (block.kind === "check" && event.key === "Backspace" && start === 0 && end === 0) {
      event.preventDefault(); change(body.slice(0, block.start) + node.value + body.slice(block.end), block.start); return;
    }
    const index = blocks.indexOf(block);
    if (event.key === "ArrowUp" && start === 0 && end === 0 && index > 0) {
      event.preventDefault(); focusRange(blocks[index - 1].end); return;
    }
    if (event.key === "ArrowDown" && start === node.value.length && end === start && index < blocks.length - 1) {
      event.preventDefault(); focusRange(blocks[index + 1].start + blocks[index + 1].prefix);
    }
  }

  let checklistNumber = 0;
  let textNumber = 0;
  return <div ref={container} className="note-live-editor" role="group" aria-label="Note editor" onClick={(event) => {
    if (event.target !== container.current) return;
    focusRange(body.length);
  }}>{blocks.map((block) => {
    const label = block.kind === "check" ? `Checklist item ${++checklistNumber}` : ++textNumber === 1 ? "Note content" : `Note text section ${textNumber}`;
    const field = <GrowingField className={`note-live-field ${block.kind === "check" && block.checked ? "completed-item" : ""}`} aria-label={label} value={block.text} maxLength={50000} placeholder={block.kind === "check" ? "Add a task..." : blocks.length === 1 ? "Start writing..." : ""} fieldRef={(node) => { if (node) fields.current.set(block.line, { node, block }); else fields.current.delete(block.line); }} onSelect={() => { lastSelection.current = selection(); }} onFocus={() => { lastSelection.current = selection(); }} onChange={(event) => {
      const node = event.currentTarget;
      if (block.kind === "check") changeCheck(block, node.value, node.selectionStart);
      else change(body.slice(0, block.start) + node.value + body.slice(block.end), block.start + node.selectionStart);
    }} onKeyDown={(event) => keyDown(event, block)}/>;
    return block.kind === "check" ? <div className={`note-live-check ${block.checked ? "checked" : ""}`} key={`check-${block.line}`}>
      <input type="checkbox" checked={block.checked} aria-label={block.text.trim() ? `Complete ${block.text}` : `Complete checklist item ${checklistNumber}`} onChange={(event) => {
        const replacement = `${event.target.checked ? "- [x] " : "- [ ] "}${block.text}`;
        change(body.slice(0, block.start) + replacement + body.slice(block.end));
      }}/>{field}
    </div> : <div className="note-live-text" key={`text-${block.line}`}>{field}</div>;
  })}</div>;
});

export default NoteBodyEditor;
