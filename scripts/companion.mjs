import { existsSync, mkdirSync } from "node:fs";
import { spawn, spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const mode = process.argv[2];
if (!["dev", "test", "build"].includes(mode)) {
  console.error("Use companion.mjs dev, companion.mjs test, or companion.mjs build.");
  process.exit(1);
}

let executable = "go";
const probe = spawnSync(executable, ["version"], { stdio: "ignore", windowsHide: true, timeout: 5000 });
if (probe.error || probe.status !== 0) {
  const candidates = process.platform === "win32" ? [path.join(process.env.ProgramFiles || "C:\\Program Files", "Go", "bin", "go.exe"), "C:\\Go\\bin\\go.exe"] : [];
  executable = candidates.find((candidate) => existsSync(candidate)) || "";
}
if (!executable) {
  console.error("Go was not found. Install Go from https://go.dev/dl/ and restart your terminal.");
  process.exit(1);
}

const directory = fileURLToPath(new URL("../companion/", import.meta.url));
const outputDir = path.join(directory, "bin");
if (mode === "build") mkdirSync(outputDir, { recursive: true });
const outputFile = path.join(outputDir, process.platform === "win32" ? "gitme-companion.exe" : "gitme-companion");
const args = mode === "dev" ? ["-C", directory, "run", "./cmd/gitme"]
  : mode === "test" ? ["-C", directory, "test", "./..."]
  : ["-C", directory, "build", "-o", outputFile, "./cmd/gitme"];
const environment = { ...process.env, GITME_APP_DIR: path.resolve(directory, "..") };
const pathKey = Object.keys(environment).find((key) => key.toLowerCase() === "path") || "PATH";
if (path.isAbsolute(executable)) environment[pathKey] = `${path.dirname(executable)}${path.delimiter}${environment[pathKey] || ""}`;
const child = spawn(executable, args, { stdio: "inherit", windowsHide: true, env: environment });
child.once("error", (error) => { console.error(`Unable to start Go: ${error.message}`); process.exitCode = 1; });
child.once("exit", (code) => {
  if (code === 0 && mode === "build") console.log(`Companion executable: ${outputFile}`);
  process.exitCode = code ?? 1;
});
