import { toast } from "@heroui/react";
import { useMutation, useMutationState } from "@tanstack/react-query";
import type { DashboardRead } from "./api";
import { useRemote } from "./hooks";
import { dashboardClient, dashboardKey } from "./query-cache";
import type { Done } from "./types";

/** Project pending edits over confirmed data. A failed write drops its projection,
 * even if the user switched pages while the request was in flight. */
export function useOptimisticSetting<T, V>(
  read: DashboardRead<T>,
  confirmed: T | undefined,
  write: (machine: string | null, value: V) => Promise<Done>,
  apply: (previous: T, value: V) => T,
) {
  const machine = useRemote();
  const key = dashboardKey(read.queryPath(machine));
  const mutationKey = ["setting", key[1]];
  const pending = useMutationState({
    filters: { mutationKey, exact: true, status: "pending" },
    select: m => m.state.variables as V,
  });
  const filter = { queryKey: key, exact: true };
  const { mutate } = useMutation({
    mutationKey,
    mutationFn: (value: V) => write(machine, value),
    onSuccess: async (answer, value) => {
      await dashboardClient.cancelQueries(filter);
      dashboardClient.setQueryData<T>(key, previous => previous === undefined ? undefined : apply(previous, value));
      toast.success(answer.message);
      await dashboardClient.invalidateQueries(filter);
    },
    onError: (err) => {
      toast.danger(err.message);
      void dashboardClient.invalidateQueries(filter);
    },
  });
  return {
    data: confirmed === undefined ? undefined : pending.reduce<T>(apply, confirmed),
    pending: pending.length > 0,
    save: (value: V) => {
      if (!dashboardClient.isMutating({ mutationKey, exact: true })) mutate(value);
    },
  };
}
