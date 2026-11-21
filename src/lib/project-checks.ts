import type { Project } from "./workspace";
import type { ServiceProfile } from "./services";
import { matchingProfile, type DetectedService, type ProjectScanResult } from "./project-scan";

export type ProjectCheck={name:string;command:string;source:"Detected"|"Saved"};
export type CheckGroup={id:string;name:string;folder:string;stack:string[];checks:ProjectCheck[];detected:boolean};
const normalize=(path:string)=>path.replace(/\\/g,"/").replace(/\/+$/,"").toLowerCase();

function savedChecks(profile:ServiceProfile,explicitOnly:boolean):ProjectCheck[]{
  return (["test","build"] as const).flatMap(slot=>{
    const command=profile.commands[slot].trim();
    if(!command||explicitOnly&&profile.commandTemplate!=="custom"&&!profile.commandOverrides?.includes(slot))return [];
    return [{name:slot==="test"?"Saved test / check":"Saved build",command,source:"Saved" as const}];
  });
}
function fromCandidate(candidate:DetectedService,profile?:ServiceProfile):ProjectCheck[]{
  const detected=(candidate.checks||[]).map(check=>({name:check.name,command:check.command,source:"Detected" as const}));
  const saved=profile?savedChecks(profile,true):[];
  return [...detected,...saved.filter(check=>!detected.some(item=>item.command===check.command))];
}
export function projectChecks(project:Project,result:ProjectScanResult|null):CheckGroup[]{
  const used=new Set<string>();
  const groups=(result?.services||[]).map(candidate=>{
    const candidatesInFolder=result!.services.filter(item=>normalize(item.folder)===normalize(candidate.folder));
    const profilesInFolder=(project.services||[]).filter(profile=>normalize(profile.folder)===normalize(candidate.folder));
    const profile=matchingProfile(project,candidate,result!)||(candidatesInFolder.length===1&&profilesInFolder.length===1?profilesInFolder[0]:undefined);
    if(profile)used.add(profile.id);
    return {id:candidate.key,name:profile?.name||candidate.name,folder:candidate.folder,stack:candidate.stack,checks:fromCandidate(candidate,profile),detected:true};
  });
  for(const profile of project.services||[]){if(!used.has(profile.id))groups.push({id:`saved:${profile.id}`,name:profile.name,folder:profile.folder,stack:profile.stack,checks:savedChecks(profile,false),detected:false});}
  return groups;
}
