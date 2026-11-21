"use client";

import { useSyncExternalStore } from "react";

function subscribe(notify: () => void) {
  window.addEventListener("online", notify);
  window.addEventListener("offline", notify);
  return () => { window.removeEventListener("online", notify); window.removeEventListener("offline", notify); };
}
const snapshot = () => navigator.onLine;
const serverSnapshot = () => null;

export default function NetworkStatus() {
  const online = useSyncExternalStore<boolean | null>(subscribe, snapshot, serverSnapshot);
  const state = online === null ? "checking" : online ? "online" : "offline";
  const label = online === null ? "Checking..." : online ? "Online" : "Offline";
  return <div className="sidebar-network" data-state={state} role="status" aria-live="polite" aria-label={`Network: ${state}`} title="Network status reported by your browser"><span aria-hidden="true"/>{label}</div>;
}
