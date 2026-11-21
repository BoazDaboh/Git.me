import { spawn } from "node:child_process";
import { access } from "node:fs/promises";
import net from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.dirname(fileURLToPath(import.meta.url));

function checkPort(port) {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", () => reject(new Error(`Port ${port} is already in use. Close the existing Git/Me instance and try again.`)));
    server.listen(port, "127.0.0.1", () => server.close(resolve));
  });
}

export async function openGitMeWindow() {
  const command = "$name = 'Git Me developer workspace.lnk'; $paths = @((Join-Path ([Environment]::GetFolderPath('Desktop')) $name), (Join-Path ([Environment]::GetFolderPath('Programs')) ('Chrome Apps\\' + $name))); foreach ($path in $paths) { if (Test-Path -LiteralPath $path -PathType Leaf) { try { Start-Process -FilePath $path -ErrorAction Stop; exit 0 } catch {} } }; exit 1";
  const opened = await new Promise((resolve) => {
    const child = spawn("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", command], { stdio: "ignore", windowsHide: true });
    child.once("error", () => resolve(false));
    child.once("exit", (code) => resolve(code === 0));
  });
  if (opened) return "app";
  const browser = spawn("rundll32.exe", ["url.dll,FileProtocolHandler", "http://127.0.0.1:3015"], { detached: true, stdio: "ignore", windowsHide: true });
  browser.once("error", () => console.error("Open http://127.0.0.1:3015 in your browser."));
  browser.unref();
  return "browser";
}

export async function launch({ appDir = root, webPort = 3015, companionPort = 47821, companionCommand, companionArgs = [], webFile, onReady } = {}) {
  const version = process.versions.node.split(".").map(Number);
  if (version[0] < 20 || (version[0] === 20 && version[1] < 9)) throw new Error("Node.js 20.9 or newer is required.");
  const web = webFile || path.join(appDir, "web", "server.js");
  const companion = companionCommand || path.join(appDir, "gitme-companion.exe");
  await Promise.all([access(web), access(companion)]);
  await checkPort(webPort);
  await checkPort(companionPort);

  const children = [];
  let stopping = false;
  function stop() {
    if (stopping) return;
    stopping = true;
    for (const child of children) if (child.exitCode === null && child.signalCode === null) child.kill();
  }
  function start(command, args, env) {
    const child = spawn(command, args, { cwd: appDir, env, stdio: "inherit", windowsHide: true });
    children.push(child);
    child.once("error", (error) => { console.error(`Git/Me failed to start: ${error.message}`); process.exitCode = 1; stop(); });
    child.once("exit", (code) => { if (!stopping) { console.error(`Git/Me service stopped (${code ?? "interrupted"}).`); process.exitCode = 1; stop(); } });
  }
  process.once("SIGINT", stop);
  process.once("SIGTERM", stop);
  start(companion, companionArgs, { ...process.env, GITME_APP_DIR: appDir });
  start(process.execPath, [web], { ...process.env, PORT: String(webPort), HOSTNAME: "127.0.0.1" });
  console.log(`Starting Git/Me at http://127.0.0.1:${webPort}. Keep this window open; press Ctrl+C to stop.`);
  onReady?.({ stop, children });
  return { stop, children };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  launch().then(async ({ stop, children }) => {
    try {
      let ready = false;
      for (let attempt = 0; attempt < 60; attempt++) {
        if (children.some((child) => child.exitCode !== null || child.signalCode !== null)) break;
        try {
          const [web, companion] = await Promise.all([
            fetch("http://127.0.0.1:3015/", { signal: AbortSignal.timeout(1500) }),
            fetch("http://127.0.0.1:47821/health", { signal: AbortSignal.timeout(1500) }),
          ]);
          const health = await companion.json();
          if (web.ok && companion.ok && health.app === "gitme-companion" && health.status === "ok") { ready = true; break; }
        } catch { /* Services may still be starting. */ }
        await new Promise((resolve) => setTimeout(resolve, 500));
      }
      if (!ready) throw new Error("Git/Me did not become ready. Check the messages above.");
      console.log("Git/Me is ready. Opening the app.");
      if (process.platform === "win32") {
        const opened = await openGitMeWindow();
        if (opened === "browser") console.log("App shortcut not found. Opened Git/Me in your browser.");
      }
    } catch (error) {
      console.error(`Could not start Git/Me: ${error.message}`);
      process.exitCode = 1;
      stop();
    }
  }).catch((error) => { console.error(`Could not start Git/Me: ${error.message}`); process.exitCode = 1; });
}
