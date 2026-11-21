import type { CvDraft } from "./workspace";

const paragraph = (text: string) => ({ type: "paragraph", content: [{ type: "text", text }] });
const entry = (title: string, date: string) => ({ type: "table", content: [{ type: "tableRow", content: [{ type: "tableCell", content: [{ type: "paragraph", content: [{ type: "text", text: title, marks: [{ type: "bold" }] }] }] }, { type: "tableCell", content: [{ type: "paragraph", attrs: { textAlign: "right" }, content: [{ type: "text", text: date }] }] }] }] });
const bullets = (...items: string[]) => ({ type: "bulletList", content: items.map(text => ({ type: "listItem", content: [paragraph(text)] })) });
const doc = (...content: object[]) => ({ type: "doc", content });
export const cvExample: CvDraft = {
 template: "classic", name: "[Your name]", role: "[Job title] | [Specialty] | [Specialty]", email: "[Your email]", phone: "[Your phone]", location: "[Your location]", website: "[Your links]",
 profile: "[Write a concise professional profile covering your experience, strengths and goals.]",
 skills: "[Skill category]: [Skills and tools]\n[Skill category]: [Skills and tools]",
 education: "[Qualification] | [Institution] | [Dates]\n[Add relevant details]",
 experience: "[Role] | [Company] | [Start date] – [End date]\n[Describe your work and impact.]\n[Add a specific project, responsibility or result]",
 projects: "[Project name] | [Project type] | [Dates]\n[Describe the project and its purpose.]\n[Add a project link when ready]\n[Project name] | [Project type] | [Dates]\n[Describe the project and its purpose.]",
 leadership: "[Add relevant leadership or community activities, or remove this section]",
 achievements: "[Add verified achievements, certifications or research, or remove this section]",
 richText: {
  experience: doc(entry("[Role] | [Company]", "[Start date] – [End date]"), bullets("[Describe your work and impact.]", "[Add a specific project, responsibility or measurable result]")),
  education: doc(entry("[Qualification] | [Institution]", "[Dates]"), bullets("[Add relevant details or remove this section]")),
  projects: doc(entry("[Project name] | [Project type]", "[Dates]"), bullets("[Describe what you built and the outcome.]", "[Add a project link when ready]"), entry("[Project name] | [Project type]", "[Dates]"), bullets("[Describe what you built and the outcome.]")),
  leadership: doc(bullets("[Add relevant leadership or community activities, or remove this section]")),
  achievements: doc(bullets("[Add verified achievements, certifications or research, or remove this section]")),
 }
};
