"use client";
import { useEffect, useState } from "react";
import Icon from "./Icon";

type BrowserNavigation = EventTarget & { canGoBack: boolean; canGoForward: boolean };

export default function WindowHistory() {
  const [available, setAvailable] = useState({ back: false, forward: false });
  useEffect(() => {
    const navigation = (window as Window & { navigation?: BrowserNavigation }).navigation;
    let frame = 0;
    const refresh = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(() => setAvailable({ back: navigation?.canGoBack ?? false, forward: navigation?.canGoForward ?? false })); };
    refresh();
    navigation?.addEventListener("currententrychange", refresh);
    window.addEventListener("popstate", refresh);
    return () => { cancelAnimationFrame(frame); navigation?.removeEventListener("currententrychange", refresh); window.removeEventListener("popstate", refresh); };
  }, []);
  return <nav className="window-history" aria-label="Page navigation"><button type="button" className="icon-button" aria-label="Go back" title="Back" disabled={!available.back} onClick={() => window.history.back()}><Icon name="arrow" size={17} style={{ transform: "rotate(180deg)" }}/></button><button type="button" className="icon-button" aria-label="Refresh page" title="Refresh" onClick={() => window.location.reload()}><Icon name="refresh" size={17}/></button><button type="button" className="icon-button" aria-label="Go forward" title="Forward" disabled={!available.forward} onClick={() => window.history.forward()}><Icon name="arrow" size={17}/></button></nav>;
}
