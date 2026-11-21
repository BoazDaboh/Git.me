import { notFound } from "next/navigation";
import Workspace from "@/components/Workspace";
import { sections, type Section } from "@/lib/workspace";

export const dynamicParams = false;
export function generateStaticParams() { return sections.filter((section) => section !== "projects").map((section) => ({ section })); }
export default async function Page({ params }: { params: Promise<{ section: string }> }) {
  const { section } = await params;
  if (!sections.some((value) => value === section)) notFound();
  return <Workspace section={section as Section} />;
}
