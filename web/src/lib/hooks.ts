import { toast } from "@heroui/react";
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { currentRemote, subscribeRemote } from "./api";
import type { Done } from "./types";

export interface Query<T> {
  data?: T;
  error?: Error;
  loading: boolean;
  reload: () => Promise<void>;
}

/** Fetches on mount, then every `every` ms while the tab is visible. */
export function useQuery<T>(load: () => Promise<T>, every = 0): Query<T> {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<Error>();
  const [loading, setLoading] = useState(true);
  const loadRef = useRef(load);
  loadRef.current = load;

  const reload = useCallback(async () => {
    try {
      setData(await loadRef.current());
      setError(undefined);
    } catch (e) {
      setError(e as Error);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    if (!every) return;
    const id = setInterval(() => {
      if (document.visibilityState === "visible") void reload();
    }, every);
    return () => clearInterval(id);
  }, [reload, every]);

  return { data, error, loading, reload };
}

let busyCount = 0;
const busyListeners = new Set<() => void>();
const setBusy = (delta: number) => {
  busyCount += delta;
  busyListeners.forEach((l) => l());
};

/** Whether any action anywhere in the dashboard is still running, for the bar across the top. */
export function useBusy() {
  return useSyncExternalStore(
    (l) => {
      busyListeners.add(l);
      return () => busyListeners.delete(l);
    },
    () => busyCount > 0,
  );
}

/** Runs a change, toasts its answer, then refreshes what it changed. */
export function useAction() {
  const [pending, setPending] = useState<string | null>(null);
  const run = useCallback(async (key: string, change: () => Promise<Done>, after?: () => unknown) => {
    setPending(key);
    setBusy(1);
    try {
      const done = await change();
      toast.success(done.message);
      await after?.();
      return true;
    } catch (e) {
      toast.danger((e as Error).message);
      return false;
    } finally {
      setPending(null);
      setBusy(-1);
    }
  }, []);
  return { pending, run };
}

const listeners = new Set<() => void>();
const notify = () => listeners.forEach((l) => l());
window.addEventListener("popstate", notify);

export function navigate(to: string) {
  if (to === window.location.pathname) return;
  window.history.pushState({ depth: historyDepth() + 1 }, "", to);
  window.scrollTo({ top: 0 });
  notify();
}

const historyDepth = (): number => window.history.state?.depth ?? 0;

/** Whether the dashboard itself has a page to go back to, so Back never leaves it. */
export const canGoBack = () => historyDepth() > 0;

const wide = window.matchMedia("(min-width: 64rem)");

/** Whether the window is wide enough for the desktop layout (Tailwind's lg). */
export function useDesktop() {
  return useSyncExternalStore(
    (l) => {
      wide.addEventListener("change", l);
      return () => wide.removeEventListener("change", l);
    },
    () => wide.matches,
  );
}

export function usePath() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => window.location.pathname,
  );
}

export type Theme = "system" | "light" | "dark";

const dark = window.matchMedia("(prefers-color-scheme: dark)");

/** Puts the chosen look on <html>; "system" follows the OS as it changes. */
export function applyTheme(theme: Theme) {
  const resolved = theme === "system" ? (dark.matches ? "dark" : "light") : theme;
  const root = document.documentElement;
  root.classList.remove("dark", "light");
  root.classList.add(resolved);
  root.dataset.theme = resolved;
}

export function storedTheme(): Theme {
  const t = localStorage.getItem("theme");
  return t === "light" || t === "dark" ? t : "system";
}

export function useTheme(): [Theme, (t: Theme) => void] {
  const [theme, setTheme] = useState<Theme>(storedTheme);
  useEffect(() => {
    applyTheme(theme);
    localStorage.setItem("theme", theme);
    if (theme !== "system") return;
    const follow = () => applyTheme("system");
    dark.addEventListener("change", follow);
    return () => dark.removeEventListener("change", follow);
  }, [theme]);
  return [theme, setTheme];
}

/** The machine whose dashboard is shown, or null for this one. */
export function useRemote() {
  return useSyncExternalStore(subscribeRemote, currentRemote);
}
