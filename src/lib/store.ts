"use client";

import { useSyncExternalStore } from "react";
import { seed, type WorkspaceData } from "./workspace";
import { isWorkspace } from "./workspace-validation";
import { normalizeWorkspace } from "./workspace-normalization";
import { mergeChanges } from "./workspace-merge";
import { migrateProjectGitHub } from "./service-github-migration";

const legacyKey = "gitme.workspace.v1";
const cacheKey = "gitme.workspace.cache.v2";
const backupKey = "gitme.workspace.before-sqlite.v1";
const endpoint = "http://127.0.0.1:47821/workspace";
const listeners = new Set<() => void>();
const statusListeners = new Set<() => void>();
type Cache = { workspace: WorkspaceData; databaseId: string | null; revision: number; pending: boolean; base?: WorkspaceData };
type Remote = { database_id: string; revision: number; database_path: string; updated_at: string; workspace: WorkspaceData | null; service_link_migration?: boolean; needsMigration?: boolean };
export type StorageStatus = { state: "loading" | "saved" | "saving" | "offline" | "conflict" | "error"; message: string; path: string; revision: number; cacheAvailable: boolean };
const initialStatus: StorageStatus = { state: "loading", message: "Connecting to SQLite", path: "", revision: 0, cacheAvailable: true };
let status = initialStatus;
let cache: Cache | null = null;
let initialized = false, busy = false, users = 0;
let generation = 0;
let timer: ReturnType<typeof setTimeout> | undefined;

function emit() { listeners.forEach((listener) => listener()); }
function setStatus(value: Partial<StorageStatus>) { status = { ...status, ...value }; statusListeners.forEach((listener) => listener()); }
function readCache() {
  if (cache) return cache;
  cache = { workspace: seed, databaseId: null, revision: 0, pending: false };
  if (typeof window === "undefined") return cache;
  try {
    const saved = JSON.parse(localStorage.getItem(cacheKey) ?? "null");
    if (saved && typeof saved === "object") {
      const normalized = normalizeWorkspace(saved.workspace);
      saved.workspace = normalized.workspace;
      if (saved.base !== undefined) saved.base = normalizeWorkspace(saved.base).workspace;
      if (normalized.changed && saved.databaseId) saved.pending = true;
    }
    if (saved && isWorkspace(saved.workspace) && (saved.databaseId === null || typeof saved.databaseId === "string") && Number.isSafeInteger(saved.revision) && saved.revision >= 0 && typeof saved.pending === "boolean") cache = saved;
    else {
      const legacy = normalizeWorkspace(JSON.parse(localStorage.getItem(legacyKey) ?? "null")).workspace as WorkspaceData | null;
      if (legacy && legacy.notes === undefined) legacy.notes = [];
      if (isWorkspace(legacy)) cache.workspace = legacy;
    }
  } catch { /* Keep the in-memory workspace if browser storage is unavailable. */ }
  if (!cache!.pending && !cache!.base) cache!.base = structuredClone(cache!.workspace);
  const migrated = migrateProjectGitHub(cache!.workspace);
  if (migrated.changed) { cache!.workspace = migrated.workspace; if (cache!.databaseId !== null) cache!.pending = true; }
  return cache!;
}
function writeCache() {
  try { const serialized = JSON.stringify(readCache()); if (localStorage.getItem(cacheKey) !== serialized) localStorage.setItem(cacheKey, serialized); setStatus({ cacheAvailable: true }); return true; }
  catch { setStatus({ cacheAvailable: false }); return false; }
}
function backupBrowser() {
  try {
    if (localStorage.getItem(backupKey) === null) localStorage.setItem(backupKey, JSON.stringify(readCache().workspace));
  } catch { /* A failed cache backup does not block database persistence. */ }
}
function schedule(delay = 600) {
  clearTimeout(timer);
  timer = setTimeout(() => { void syncWorkspace(); }, delay);
}
function conflict() {
  setStatus({ state: "conflict", message: "Newer database changes found. Export your pending edits, then load the database copy in Settings." });
}

async function request(method = "GET", body?: unknown) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 6000);
  try {
    return await fetch(endpoint, { method, signal: controller.signal, cache: "no-store", headers: method === "GET" ? { Accept: "application/json" } : { "Content-Type": "application/json", "X-GitMe-Client": "workspace-v1" }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  } finally { clearTimeout(timeout); }
}

async function remoteSnapshot(): Promise<Remote> {
  const response = await request();
  if (!response.ok) throw new Error(`Database read returned HTTP ${response.status}.`);
  const remote = await response.json();
  const normalized = normalizeWorkspace(remote?.workspace);
  if (!remote || typeof remote.database_id !== "string" || !remote.database_id || !Number.isSafeInteger(remote.revision) || remote.revision < 0 || typeof remote.database_path !== "string" || (remote.revision === 0 ? normalized.workspace !== null : !isWorkspace(normalized.workspace))) throw new Error("The companion returned an incompatible workspace. Check the saved workspace format.");
  remote.workspace = normalized.workspace;
  remote.needsMigration = normalized.changed || !!remote.service_link_migration;
  if (remote.workspace) { const migrated = migrateProjectGitHub(remote.workspace); remote.workspace = migrated.workspace; remote.needsMigration ||= migrated.changed; }
  return remote;
}

export async function syncWorkspace() {
  if (busy || status.state === "conflict") return;
  busy = true;
  try {
    readCache();
    const remote = await remoteSnapshot();
    const current = readCache();
    setStatus({ path: remote.database_path });
    if (remote.revision === 0) {
      if (current.databaseId && current.databaseId !== remote.database_id && current.pending) { conflict(); return; }
      backupBrowser();
      current.databaseId = remote.database_id; current.revision = 0; current.pending = true;
      writeCache();
    } else if (!current.pending) {
      backupBrowser();
      const changed = !initialized || current.databaseId !== remote.database_id || current.revision !== remote.revision || !!remote.needsMigration;
      if (changed) cache = { workspace: remote.workspace!, base: structuredClone(remote.workspace!), databaseId: remote.database_id, revision: remote.revision, pending: !!remote.needsMigration };
      initialized = true;
      writeCache();
      if (changed) emit();
      if (!remote.needsMigration) { setStatus({ state: "saved", message: "Saved to SQLite", revision: remote.revision }); return; }
    } else if (current.databaseId !== remote.database_id || current.revision !== remote.revision) {
      if (current.databaseId !== remote.database_id || !current.base || !remote.workspace) { conflict(); return; }
      const merged = mergeChanges(current.base, current.workspace, remote.workspace);
      if (!merged.ok || !isWorkspace(merged.value)) { conflict(); return; }
      current.workspace = merged.value; current.base = structuredClone(remote.workspace); current.revision = remote.revision; generation++;
      if (JSON.stringify(current.workspace) === JSON.stringify(remote.workspace)) { current.pending = false; initialized = true; writeCache(); emit(); setStatus({ state: "saved", message: "Saved to SQLite", revision: remote.revision }); return; }
      writeCache(); emit();
    }

    const saving = structuredClone(readCache());
    const savedGeneration = generation;
    setStatus({ state: "saving", message: "Saving to SQLite" });
    const response = await request("PUT", { database_id: saving.databaseId, revision: saving.revision, workspace: saving.workspace });
    if (response.status === 409) { setStatus({ state: "saving", message: "Reconciling workspace changes" }); schedule(0); return; }
    if (!response.ok) { setStatus({ state: "error", message: `Save failed (HTTP ${response.status}). Your edits remain in this browser; export a copy or retry.` }); return; }
    const saved = await response.json();
    if (saved.database_id !== saving.databaseId || saved.revision !== saving.revision + 1) throw new Error("Invalid save acknowledgement. Your edits are still pending.");
    const latest = readCache();
    latest.databaseId = saved.database_id; latest.revision = saved.revision; latest.base = structuredClone(saving.workspace);
    latest.pending = generation !== savedGeneration;
    initialized = true;
    writeCache();
    setStatus({ state: latest.pending ? "saving" : "saved", message: latest.pending ? "Saving to SQLite" : "Saved to SQLite", revision: saved.revision });
    if (latest.pending) schedule(0);
  } catch (error) {
    const detail = error instanceof Error && error.message.includes("incompatible") ? error.message : "Companion unavailable. Changes are pending in this browser; keep it open or return here after restarting Go.";
    setStatus({ state: "offline", message: detail });
  } finally { busy = false; if (readCache().pending && status.state === "saving") schedule(); }
}

export async function loadDatabaseCopy() {
  if (busy) return;
  busy = true;
  try {
    const remote = await remoteSnapshot();
    if (remote.workspace === null) throw new Error("Database is empty.");
    backupBrowser();
    cache = { workspace: remote.workspace, base: structuredClone(remote.workspace), databaseId: remote.database_id, revision: remote.revision, pending: !!remote.needsMigration };
    generation++; initialized = true;
    writeCache(); emit();
    setStatus({ state: "saved", message: "Saved to SQLite", revision: remote.revision, path: remote.database_path });
    if (remote.needsMigration) schedule(0);
  } catch { setStatus({ state: "conflict", message: "Could not load the database copy. Keep your pending edits and retry when the companion is available." }); }
  finally { busy = false; }
}

export function startWorkspaceSync() {
  users++;
  const poll = () => { if (document.visibilityState === "visible") void syncWorkspace(); };
  const onStorage = (event: StorageEvent) => {
    if (event.key !== cacheKey && event.key !== null) return;
    const current = readCache();
    if (current.pending) {
      try {
        const incoming = event.newValue ? JSON.parse(event.newValue) : null;
        if (incoming && isWorkspace(incoming.workspace) && incoming.databaseId === current.databaseId && incoming.revision === current.revision && JSON.stringify(incoming.workspace) === JSON.stringify(current.workspace)) return;
        if (incoming && isWorkspace(incoming.workspace) && incoming.databaseId === current.databaseId && incoming.revision === current.revision && current.base && incoming.base && JSON.stringify(incoming.base) === JSON.stringify(current.base)) {
          const merged = mergeChanges(current.base, current.workspace, incoming.workspace);
          if (merged.ok && isWorkspace(merged.value)) { if (JSON.stringify(merged.value) !== JSON.stringify(current.workspace)) { current.workspace = merged.value; generation++; writeCache(); emit(); } if (!busy) schedule(); return; }
        }
      } catch { /* Keep the pending copy if another window has invalid cache data. */ }
      if (!busy) schedule(0); return;
    }
    if (busy) return;
    cache = null; readCache(); generation++; emit(); poll();
  };
  const interval = setInterval(poll, 5000);
  window.addEventListener("focus", poll); window.addEventListener("online", poll); window.addEventListener("storage", onStorage); window.addEventListener("gitme:companion-ready", poll); document.addEventListener("visibilitychange", poll);
  void syncWorkspace();
  return () => { users--; clearInterval(interval); window.removeEventListener("focus", poll); window.removeEventListener("online", poll); window.removeEventListener("storage", onStorage); window.removeEventListener("gitme:companion-ready", poll); document.removeEventListener("visibilitychange", poll); if (users === 0) clearTimeout(timer); };
}

export function updateWorkspace(update: (data: WorkspaceData) => WorkspaceData) {
  const current = readCache();
  current.workspace = migrateProjectGitHub(update(current.workspace)).workspace; current.pending = true; generation++;
  const persisted = writeCache();
  emit();
  if (status.state !== "conflict") { setStatus({ state: "saving", message: "Pending SQLite save" }); schedule(); }
  return persisted;
}
const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
const subscribeStatus = (listener: () => void) => { statusListeners.add(listener); return () => { statusListeners.delete(listener); }; };
export function useWorkspace() { return useSyncExternalStore(subscribe, () => readCache().workspace, () => seed); }
export function useWorkspaceStorage() { return useSyncExternalStore(subscribeStatus, () => status, () => initialStatus); }
export function resetWorkspace() { return updateWorkspace(() => structuredClone(seed)); }
export function importWorkspace(value: unknown) {
  const normalized = normalizeWorkspace(value).workspace;
  if (!isWorkspace(normalized)) throw new Error("This is not a supported Git/Me workspace export.");
  return updateWorkspace(() => normalized);
}
