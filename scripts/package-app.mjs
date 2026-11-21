import { access, cp, mkdir, mkdtemp, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawn } from "node:child_process";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const web = path.join(root, ".next", "gitme-web");
const companion = path.join(root, "companion", "bin", "gitme-companion.exe");
await Promise.all([access(path.join(web, "server.js")), access(companion)]);
const packages = path.join(root, "dist");
await mkdir(packages, { recursive: true });
const output = await mkdtemp(path.join(packages, "gitme-app-"));
await cp(web, path.join(output, "web"), { recursive: true, dereference: true });
await cp(companion, path.join(output, "gitme-companion.exe"));
await cp(path.join(root, "scripts", "start-production.mjs"), path.join(output, "start-production.mjs"));
await writeFile(path.join(output, "Start GitMe.cmd"), '@echo off\r\nwhere node >nul 2>&1\r\nif errorlevel 1 (\r\n  echo Node.js is required. Install Node.js and reopen this window.\r\n  pause\r\n  exit /b 1\r\n)\r\nnode "%~dp0start-production.mjs"\r\nif errorlevel 1 pause\r\n');
console.log(`Git/Me package: ${output}`);
if (process.argv.includes("--installer")) {
  const compiler = path.join(process.env.LOCALAPPDATA || "", "Programs", "Inno Setup 6", "ISCC.exe");
  const candidates = [compiler, "C:\\Program Files (x86)\\Inno Setup 6\\ISCC.exe", "C:\\Program Files\\Inno Setup 6\\ISCC.exe"];
  let executable = "";
  for (const candidate of candidates) {
    try { await access(candidate); executable = candidate; break; } catch { /* Try the next standard location. */ }
  }
  if (!executable) throw new Error("Inno Setup 6 was not found. Install it before building the installer.");
  await new Promise((resolve, reject) => {
    const child = spawn(executable, [`/DSourceDir=${output}`, path.join(root, "installer", "gitme.iss")], { stdio: "inherit", windowsHide: true });
    child.once("error", reject);
    child.once("exit", (code) => code === 0 ? resolve() : reject(new Error(`Installer build failed (${code})`)));
  });
}
