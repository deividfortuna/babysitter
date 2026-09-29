import { useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { settingsQueryKey, watchListQueryKey } from "../lib/query-keys";

export type Settings = components["schemas"]["HttpdSettings"];

const RETRY_AFTER_MS = 30_000;

export function useSettings(enabled = true) {
  return useQuery({
    queryKey: settingsQueryKey,
    enabled,
    staleTime: Infinity,
    refetchInterval: (query) => (query.state.status === "error" ? RETRY_AFTER_MS : false),
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/settings");
      if (error) throw new Error(apiErrorMessage(error, "Could not load the settings."));
      return data;
    },
  });
}

export function useWriteSettings() {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    scope: { id: "settings" },
    mutationFn: async (patch: Partial<Settings>) => {
      const body = { ...queryClient.getQueryData<Settings>(settingsQueryKey), ...patch } as Settings;
      const { data, error } = await api().PUT("/api/v1/settings", { body });
      if (error) throw new Error(apiErrorMessage(error, "Could not save the settings."));
      return data;
    },
    onMutate: async (patch) => {
      await queryClient.cancelQueries({ queryKey: settingsQueryKey });
      const before = queryClient.getQueryData<Settings>(settingsQueryKey);
      if (before) queryClient.setQueryData(settingsQueryKey, { ...before, ...patch });
      return { before };
    },
    onError: (_error, _patch, context) => {
      if (context?.before) queryClient.setQueryData(settingsQueryKey, context.before);
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(settingsQueryKey, saved);
      void queryClient.invalidateQueries({ queryKey: watchListQueryKey("active") });
      void queryClient.invalidateQueries({ queryKey: watchListQueryKey("all") });
    },
  });
  const { mutateAsync } = mutation;
  const write = useCallback((patch: Partial<Settings>) => mutateAsync(patch), [mutateAsync]);
  return { write, error: mutation.error };
}
