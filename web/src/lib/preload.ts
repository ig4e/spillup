import { api, currentRemote, type DashboardRead } from "./api";
import { prefetchQuery } from "./hooks";

// Only read data the destination displays immediately. Diagnostics, credentials,
// provider quota requests, streams and terminals stay behind their own controls.
const pageReads: Record<string, DashboardRead<unknown>[]> = {
  "/": [api.machines, api.skills],
  "/machines": [api.machines],
  "/activity": [api.localMachines],
  "/more": [api.localMachines, api.localOverview],
  "/tailscale": [api.machines],
  "/skills": [api.skills],
  "/accounts": [api.apps],
  "/sign-in": [api.own],
  "/proxy": [api.own, api.localOverview],
  "/t3": [api.machines],
  "/desktop": [api.localMachines],
  "/terminal": [api.localMachines, api.localOverview],
};

function mayPreload() {
  const connection = (navigator as Navigator & {
    connection?: { saveData?: boolean; effectiveType?: string };
  }).connection;
  return document.visibilityState === "visible" && navigator.onLine &&
    !connection?.saveData && !connection?.effectiveType?.includes("2g");
}

/** Installed after the selected machine has returned an authenticated overview. */
export function startNavigationPreload() {
  const machine = currentRemote();
  const allowed = () => currentRemote() === machine && mayPreload();
  const warmLink = (event: Event) => {
    if (!allowed() || !(event.target instanceof Element)) return;
    const link = event.target.closest<HTMLAnchorElement>("a[href]");
    if (!link || link.origin !== location.origin || link.pathname === location.pathname) return;
    const targetMachine = new URL(link.href).searchParams.get("machine");
    if (targetMachine && targetMachine !== machine) return;
    for (const read of pageReads[link.pathname] ?? []) void prefetchQuery(read);
  };

  const warmTabs = () => {
    if (allowed()) void prefetchQuery(api.machines);
  };
  const useIdle = typeof window.requestIdleCallback === "function";
  const id = useIdle ? window.requestIdleCallback(warmTabs, { timeout: 1500 }) : window.setTimeout(warmTabs, 300);

  document.addEventListener("focusin", warmLink);
  document.addEventListener("pointerover", warmLink, { passive: true });
  document.addEventListener("pointerdown", warmLink, { passive: true });
  return () => {
    if (useIdle) window.cancelIdleCallback(id);
    else window.clearTimeout(id);
    document.removeEventListener("focusin", warmLink);
    document.removeEventListener("pointerover", warmLink);
    document.removeEventListener("pointerdown", warmLink);
  };
}
