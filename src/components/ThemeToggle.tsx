"use client";

import { useEffect, useSyncExternalStore } from "react";
import Icon from "./Icon";

type Theme = "light" | "dark";
const key = "gitme.theme";
const listeners = new Set<() => void>();
function current(): Theme { return document.documentElement.dataset.theme === "dark" ? "dark" : "light"; }
function apply(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  document.querySelectorAll('meta[name="theme-color"]').forEach((meta) => meta.setAttribute("content", theme === "dark" ? "#101010" : "#ffffff"));
}
function subscribe(listener: () => void) {
  listeners.add(listener);
  const onStorage = (event: StorageEvent) => {
    if (event.key !== key) return;
    apply(event.newValue === "dark" ? "dark" : "light");
    listeners.forEach((notify) => notify());
  };
  window.addEventListener("storage", onStorage);
  return () => { listeners.delete(listener); window.removeEventListener("storage", onStorage); };
}
export default function ThemeToggle() {
  const theme = useSyncExternalStore(subscribe, current, () => "light");
  useEffect(() => { apply(current()); }, []);
  function toggle() {
    const next = theme === "dark" ? "light" : "dark";
    apply(next);
    try { localStorage.setItem(key, next); } catch { /* Keep the theme usable without storage. */ }
    listeners.forEach((notify) => notify());
  }
  return <button className="icon-button theme-toggle" aria-label={`Switch to ${theme === "light" ? "dark" : "light"} mode`} title={`Switch to ${theme === "light" ? "dark" : "light"} mode`} onClick={toggle}><Icon name={theme === "light" ? "moon" : "sun"} size={17}/></button>;
}
