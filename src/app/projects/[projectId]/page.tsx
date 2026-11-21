import Workspace from "@/components/Workspace";

export default async function Page({ params, searchParams }: { params: Promise<{ projectId: string }>; searchParams:Promise<{newService?:string|string[]}> }) {
  const { projectId } = await params;
  const createService=(await searchParams).newService==="1";
  return <Workspace section="projects" projectId={projectId} createService={createService}/>;
}
