"use client";
import { useEffect } from "react";
import { EditorContent, useEditor, type Editor, type JSONContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { TextStyle, FontSize, Color } from "@tiptap/extension-text-style";
import TextAlign from "@tiptap/extension-text-align";
import { TableKit } from "@tiptap/extension-table";

function plainDocument(text: string): JSONContent { return { type: "doc", content: text.split("\n").map(line => ({ type: "paragraph", ...(line ? { content: [{ type: "text", text: line }] } : {}) })) }; }
export default function CvRichField({ field, label, value, content, className, onFocus, onChange }: { field: string; label: string; value: string; content?: unknown; className?: string; onFocus: (editor: Editor, field: string) => void; onChange: (field: string, plain: string, rich: JSONContent) => void }) {
  const editor = useEditor({ immediatelyRender: false, extensions: [StarterKit.configure({ heading: { levels: [2, 3] }, codeBlock: false, blockquote: false, horizontalRule: false, link: { openOnClick: false } }), TextStyle, FontSize, Color, TableKit, TextAlign.configure({ types: ["paragraph", "heading"] })], content: (content as JSONContent | undefined) ?? plainDocument(value), editorProps: { attributes: { "aria-label": label, role: "textbox", "aria-multiline": "true", class: `cv-rich-editor ${className ?? ""}` } }, onFocus: ({ editor }) => onFocus(editor, field), onUpdate: ({ editor }) => onChange(field, editor.getText({ blockSeparator: "\n" }), editor.getJSON()) });
  useEffect(() => { if (!editor || editor.isFocused) return; const wanted = (content as JSONContent | undefined) ?? plainDocument(value); if (JSON.stringify(editor.getJSON()) !== JSON.stringify(wanted)) editor.commands.setContent(wanted, { emitUpdate: false }); }, [editor, content, value]);
  return <EditorContent editor={editor}/>;
}
