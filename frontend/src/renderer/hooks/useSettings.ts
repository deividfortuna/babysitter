import { useCallback } from "react";
import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { confirmedSettingsQueryKey, settingsMutationKey, settingsQueryKey, watchListQueryKey } from "../lib/query-keys";

export type Settings = components["schemas"]["HttpdSettings"];

type Patch = Partial<Settings>;

const RETRY_AFTER_MS = 30_000;

export function useSettings(enabled = true) {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: settingsQueryKey,
    enabled,
    staleTime: Infinity,
    refetchInterval: (query) => (query.state.status === "error" ? RETRY_AFTER_MS : false),
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/settings");
      if (error) throw new Error(apiErrorMessage(error, "Could not load the settings."));
      queryClient.setQueryData(confirmedSettingsQueryKey, data);
      return data;
    },
  });
}

function confirmedSettings(queryClient: QueryClient): Settings | undefined {
  return queryClient.getQueryData<Settings>(confirmedSettingsQueryKey);
}

function otherPendingPatches(queryClient: QueryClient, own: Patch): Patch[] {
  return queryClient
    .getMutationCache()
    .findAll({ mutationKey: settingsMutationKey, status: "pending" })
    .map((mutation) => mutation.state.variables as Patch)
    .filter((patch) => patch !== own);
}

export function useWriteSettings() {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationKey: settingsMutationKey,
    scope: { id: "settings" },
    mutationFn: async (patch: Patch) => {
      const body = { ...confirmedSettings(queryClient), ...patch } as Settings;
      const { data, error } = await api().PUT("/api/v1/settings", { body });
      if (error) throw new Error(apiErrorMessage(error, "Could not save the settings."));
      return data;
    },
    onMutate: async (patch) => {
      await queryClient.cancelQueries({ queryKey: settingsQueryKey });
      const shown = queryClient.getQueryData<Settings>(settingsQueryKey);
      if (!shown) return;
      if (!queryClient.getQueryData(confirmedSettingsQueryKey))
        queryClient.setQueryData(confirmedSettingsQueryKey, shown);
      queryClient.setQueryData(settingsQueryKey, { ...shown, ...patch });
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(confirmedSettingsQueryKey, saved);
      void queryClient.invalidateQueries({ queryKey: watchListQueryKey("active") });
      void queryClient.invalidateQueries({ queryKey: watchListQueryKey("all") });
    },
    onSettled: (_saved, _error, patch) => {
      const confirmed = confirmedSettings(queryClient);
      if (!confirmed) return;
      const pending = otherPendingPatches(queryClient, patch);
      queryClient.setQueryData(settingsQueryKey, Object.assign({}, confirmed, ...pending));
    },
  });
  const { mutateAsync } = mutation;
  const write = useCallback((patch: Patch) => mutateAsync(patch), [mutateAsync]);
  return { write, error: mutation.error };
}
