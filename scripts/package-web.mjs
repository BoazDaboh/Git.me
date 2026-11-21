import { access, cp, mkdir } from "node:fs/promises";
import { spawn } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const standalone = path.join(root, ".next", "standalone");
const output = path.join(root, ".next", "gitme-web");

await access(path.join(standalone, "server.js"));
await mkdir(output, { recursive: true });
for (const file of ["package.json", "package-lock.json"]) {
  await cp(path.join(root, file), path.join(output, file));
}

const windows = process.platform === "win32";
const npm = windows ? (process.env.ComSpec || "cmd.exe") : "npm";
const args = ["ci", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund"];
await new Promise((resolve, reject) => {
  const child = spawn(npm, windows ? ["/d", "/s", "/c", "npm.cmd", ...args] : args, {
    cwd: output,
    stdio: "inherit",
  });
  child.on("error", reject);
  child.on("exit", (code) => code === 0 ? resolve() : reject(new Error(`npm ci failed (${code})`)));
});

await cp(path.join(standalone, "server.js"), path.join(output, "server.js"));
await cp(path.join(standalone, ".next"), path.join(output, ".next"), { recursive: true, dereference: true });
await cp(path.join(root, ".next", "static"), path.join(output, ".next", "static"), { recursive: true });
await cp(path.join(root, "public"), path.join(output, "public"), { recursive: true });
console.log("Production web package ready in .next/gitme-web");
