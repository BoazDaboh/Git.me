import type { ServiceCommand, ServiceProfile, ServiceRole } from "./services";

export type CommandTemplate = "auto" | "next" | "node" | "frontend" | "browser" | "php" | "composer" | "laravel" | "go" | "custom";
export type CommandSuggestion = { template: CommandTemplate; title: string; runtime: string; commands: Record<ServiceCommand, string>; hint: string; installHint?: string };
export const commandTemplates: { value: CommandTemplate; label: string }[] = [
  { value: "auto", label: "Automatic, from selected stack" }, { value: "next", label: "Next.js" },
  { value: "frontend", label: "Frontend with npm scripts" }, { value: "node", label: "Node.js with npm scripts" },
  { value: "browser", label: "Plain browser JavaScript" }, { value: "php", label: "Plain PHP" },
  { value: "composer", label: "PHP with Composer" }, { value: "laravel", label: "Laravel" },
  { value: "go", label: "Go API" }, { value: "custom", label: "Custom commands" },
];
const commandKeys: ServiceCommand[] = ["install", "develop", "build", "test", "start"];
const normalize = (value: string) => value.toLowerCase().replace(/[^a-z0-9]/g, "");
const empty = (): Record<ServiceCommand, string> => ({ install: "", develop: "", build: "", test: "", start: "" });

function inferTemplate(stack: string[], runtime: string, role: ServiceRole): CommandTemplate {
  const tags = new Set(stack.map(normalize));
  const has = (...names: string[]) => names.some((name) => tags.has(name));
  if (role === "frontend" && has("nextjs", "next")) return "next";
  if (has("laravel")) return "laravel";
  if (has("php", "composer")) return has("composer") ? "composer" : "php";
  if (has("go", "golang")) return "go";
  if (has("nextjs", "next")) return "next";
  if (has("nodejs", "node", "nestjs", "nest", "express", "expressjs")) return "node";
  if (has("react", "vite", "vue", "nuxt", "svelte", "astro")) return "frontend";
  if (has("javascript", "js", "typescript", "ts", "html", "css")) return role === "backend" || role === "worker" ? "node" : "browser";
  if (tags.size) return "custom";
  const engine = normalize(runtime);
  if (/^php\d*/.test(engine)) return "php";
  if (/^(go|golang)(\d|$)/.test(engine)) return "go";
  if (/^node(js)?(\d|$)/.test(engine)) return "node";
  return "custom";
}

export function suggestServiceCommands(service: Pick<ServiceProfile, "stack" | "runtime" | "role" | "port" | "commandTemplate">): CommandSuggestion {
  const template = service.commandTemplate && service.commandTemplate !== "auto" ? service.commandTemplate : inferTemplate(service.stack, service.runtime, service.role);
  const validPort = service.port && service.port >= 1 && service.port <= 65535 ? service.port : 8080;
  if (template === "next") return { template, title: "Next.js", runtime: "Node.js", commands: { install: "npm.cmd install", develop: "npm.cmd run dev", build: "npm.cmd run build", test: "npm.cmd run lint", start: "npm.cmd run start" }, hint: "Suggested npm scripts. Confirm they exist in package.json; files have not been scanned." };
  if (template === "frontend") return { template, title: "Frontend with npm scripts", runtime: "Node.js", commands: { install: "npm.cmd install", develop: "npm.cmd run dev", build: "npm.cmd run build", test: "", start: "" }, hint: "Assumes an npm-based build tool with dev and build scripts. Confirm them in package.json." };
  if (template === "node") return { template, title: "Node.js with npm scripts", runtime: "Node.js", commands: { install: "npm.cmd install", develop: "npm.cmd run dev", build: "", test: "npm.cmd test", start: "npm.cmd start" }, hint: "Requires matching package.json scripts. Add a build command if your backend needs one." };
  if (template === "php" || template === "composer") return { template, title: template === "composer" ? "PHP with Composer" : "Plain PHP", runtime: "PHP", commands: { ...empty(), install: template === "composer" ? "composer install" : "", develop: `php -S 127.0.0.1:${validPort}` }, hint: template === "composer" ? "Composer requires composer.json. The PHP server serves this folder for local development only." : "Plain PHP needs no dependency install. Add Composer to the stack if you use composer.json. The server serves this folder locally.", installHint: template === "php" ? "Not needed for plain PHP" : undefined };
  if (template === "laravel") return { template, title: "Laravel", runtime: "PHP", commands: { ...empty(), install: "composer install", develop: `php artisan serve --host=127.0.0.1 --port=${validPort}`, test: "php artisan test" }, hint: "Run from the folder containing artisan. Frontend asset commands can use a separate frontend profile." };
  if (template === "go") return { template, title: "Go API", runtime: "Go", commands: { install: "go mod download", develop: "go run ./cmd/api", build: "go build -o bin/api.exe ./cmd/api", test: "go test ./...", start: ".\\bin\\api.exe" }, hint: "Assumes your entry point is ./cmd/api. Change it if your Go project uses a different layout." };
  if (template === "browser") return { template, title: "Plain browser JavaScript", runtime: "Browser", commands: empty(), hint: "Browser JavaScript has no standard install or dev-server command. Choose your build tool or enter your own commands.", installHint: "Not needed without a package manager" };
  return { template: "custom", title: "Custom commands", runtime: "", commands: empty(), hint: "Enter the commands your project uses, or choose a command template." };
}

export function serviceStackForRole(stack: string[], role: ServiceRole): string[] {
  const tags = stack.map(normalize);
  if (role === "backend") {
    if (tags.some((tag) => ["go", "golang"].includes(tag))) return stack.filter((item) => ["go", "golang"].includes(normalize(item)));
    if (tags.some((tag) => ["php", "laravel", "composer"].includes(tag))) return stack.filter((item) => ["php", "laravel", "composer"].includes(normalize(item)));
    return stack.filter((item) => ["nodejs", "node", "nestjs", "express", "javascript", "js", "typescript", "ts", "python", "django", "fastapi"].includes(normalize(item)));
  }
  if (role === "frontend") {
    const frontend = stack.filter((item) => ["nextjs", "next", "react", "vite", "vue", "nuxt", "svelte", "astro", "javascript", "js", "typescript", "ts", "html", "css", "tailwindcss", "nodejs"].includes(normalize(item)));
    return frontend.length ? frontend : stack.filter((item) => ["php", "laravel", "composer"].includes(normalize(item)));
  }
  return [];
}

function overridesFor(service: ServiceProfile): ServiceCommand[] {
  if (Array.isArray(service.commandOverrides)) return service.commandOverrides.filter((key) => commandKeys.includes(key));
  const legacyNext = { install: "npm.cmd ci", develop: "npm.cmd run dev", build: "npm.cmd run build", test: "npm.cmd run lint", start: "npm.cmd run start" };
  const legacyGo = { install: "", develop: "go run ./cmd/api", build: "go build -o bin/api.exe ./cmd/api", test: "go test ./...", start: ".\\bin\\api.exe" };
  const suggestion = suggestServiceCommands(service);
  const candidates = [suggestion.commands, legacyNext, legacyGo].map((commands) => ({ commands, matches: commandKeys.filter((key) => service.commands[key] === commands[key]).length })).sort((a, b) => b.matches - a.matches);
  const baseline = candidates[0].matches >= 3 ? candidates[0].commands : suggestion.commands;
  return commandKeys.filter((key) => service.commands[key] !== baseline[key]);
}

export function refreshServiceSuggestions(service: ServiceProfile, force = false): ServiceProfile {
  const suggestion = suggestServiceCommands(service);
  const overrides = force ? [] : overridesFor(service);
  if (service.commandTemplate === "custom" && !force) return { ...service, commandOverrides: [...commandKeys] };
  const commands = { ...service.commands };
  for (const key of commandKeys) if (!overrides.includes(key)) commands[key] = suggestion.commands[key];
  const autoUnknown = suggestion.template === "custom" && (service.commandTemplate ?? "auto") === "auto" && service.stack.length > 0;
  const runtime = !service.runtime || ["Node.js", "PHP", "Go", "Browser"].includes(service.runtime) ? suggestion.runtime || (autoUnknown ? "" : service.runtime) : service.runtime;
  return { ...service, runtime, commands, commandTemplate: service.commandTemplate ?? "auto", commandOverrides: overrides };
}

export function editServiceCommand(service: ServiceProfile, key: ServiceCommand, command: string): ServiceProfile {
  return { ...service, commands: { ...service.commands, [key]: command }, commandOverrides: [...new Set([...overridesFor(service), key])] };
}
