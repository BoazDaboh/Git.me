type Refresh = (force?: boolean) => Promise<void>;

export function watchConnectionRefresh(refresh: Refresh, pollMs: number, storageKey?: string) {
  let active = true;
  let lastWake = 0;
  let wakeTimer: ReturnType<typeof setTimeout> | undefined;
  let recoveryTimer: ReturnType<typeof setTimeout> | undefined;
  const visible = () => document.visibilityState === "visible";
  const wake = () => {
    if (!active || !visible() || wakeTimer !== undefined) return;
    const delay = storageKey ? Math.max(0, 10000 - (Date.now() - lastWake)) : 0;
    wakeTimer = setTimeout(() => {
      wakeTimer = undefined;
      if (!active || !visible()) return;
      lastWake = Date.now();
      void refresh(true);
    }, delay);
  };
  const online = () => {
    wake();
    if (storageKey) {
      clearTimeout(recoveryTimer);
      recoveryTimer = setTimeout(wake, 11000);
    }
  };
  const storage = (event: StorageEvent) => { if (storageKey && event.key === storageKey) void refresh(true); };
  const poll = () => { if (visible()) void refresh(); };
  const timer = setInterval(poll, pollMs);
  window.addEventListener("online", online);
  window.addEventListener("focus", wake);
  window.addEventListener("storage", storage);
  document.addEventListener("visibilitychange", wake);
  if (storageKey) window.addEventListener("gitme:companion-ready", online);
  return {
    wake,
    stop() {
      active = false; clearInterval(timer); clearTimeout(wakeTimer); clearTimeout(recoveryTimer);
      window.removeEventListener("online", online); window.removeEventListener("focus", wake); window.removeEventListener("storage", storage);
      document.removeEventListener("visibilitychange", wake);
      if (storageKey) window.removeEventListener("gitme:companion-ready", online);
    },
  };
}
