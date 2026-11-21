import { spawn } from "node:child_process";
import { mkdirSync, openSync } from "node:fs";
import http from "node:http";
import net from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const logs = path.join(process.env.LOCALAPPDATA || root, "GitMe", "logs");
mkdirSync(logs, { recursive: true });
function occupied(port) {
  return new Promise(resolve => {
    const socket = net.createConnection({ host: "127.0.0.1", port });
    socket.setTimeout(1200);
    socket.once("connect", () => { socket.destroy(); resolve(true); });
    socket.once("timeout", () => { socket.destroy(); resolve(false); });
    socket.once("error", () => resolve(false));
  });
}
function launch(name, args) {
  const out = openSync(path.join(logs, `${name}.log`), "a");
  const child = spawn(process.execPath, args, { cwd: root, detached: true, windowsHide: true, stdio: ["ignore", out, out], env: process.env });
  child.on("error", () => {});
  child.unref();
}
if (!(await occupied(47821))) launch("companion", [path.join(root, "scripts", "companion.mjs"), "dev"]);
if (!(await occupied(3015))) launch("web", [path.join(root, "node_modules", "next", "dist", "bin", "next"), "dev", "--hostname", "127.0.0.1", "--port", "3015"]);
