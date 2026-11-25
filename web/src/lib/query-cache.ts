import { QueryClient } from "@tanstack/react-query";

export const defaultFreshness = 15_000;

export function createDashboardClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { staleTime: defaultFreshness, gcTime: 5 * 60_000, retry: false },
      // A dashboard command can have external effects. Never replay it on reconnect.
      mutations: { retry: false, networkMode: "always" },
    },
  });
}

export const dashboardClient = createDashboardClient();
export const dashboardKey = (path: string) => ["dashboard", path] as const;

export function clearDeniedData(client: QueryClient, path: string, error: Error & { status: number }) {
  if (error.status !== 401 && error.status !== 403) return;
  const peer = path.match(/^remote\/[^/]+\//)?.[0];
  const endpoint = peer ? path.slice(peer.length) : path;
  const session = error.message === "open the dashboard with `spinup ui` to sign in"
    || error.message === "spillup's dashboard answers only devices signed in to your Tailscale account";
  const scopeDenied = endpoint === "overview" || (!!peer && error.message === "missing or wrong key for this machine's dashboard");
  for (const query of client.getQueryCache().findAll({ queryKey: ["dashboard"] })) {
    const cachedPath = String(query.queryKey[1]);
    if (!session && cachedPath !== path && !(scopeDenied && (!peer || cachedPath.startsWith(peer)))) continue;
    // Reset cancels in-flight reads without refetching a denied endpoint. Notify
    // mounted observers immediately so protected data disappears even on a warm view.
    query.reset();
    query.setState({ status: "error", error, errorUpdatedAt: Date.now() });
  }
}

export async function invalidateDashboard() {
  const filter = { queryKey: ["dashboard"] };
  await dashboardClient.cancelQueries(filter);
  await dashboardClient.invalidateQueries(filter);
}
