import { useMemo } from "react";
import { useMutation, useMutationState, useQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { settingsMutationKey, settingsQueryKey, watchListQueryKey } from "../lib/query-keys";

export type Settings = components["schemas"]["HttpdSettings"];

type Patch = Partial<Settings>;

const RETRY_AFTER_MS = 30_000;

export function useSettings(enabled = true) {
  const query = useQuery({
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
  const saving = useMutationState({
    filters: { mutationKey: settingsMutationKey, status: "pending" },
    select: (mutation) => mutation.state.variables as Patch,
  });
  const { data: confirmed, error, isError, isSuccess } = query;
  const data = useMemo(() => confirmed && (Object.assign({}, confirmed, ...saving) as Settings), [confirmed, saving]);
  return { data, error, isError, isSuccess };
}

export function useWriteSettings() {
  const queryClient = useQueryClient();
  const { mutateAsync } = useMutation({
    mutationKey: settingsMutationKey,
    scope: { id: "settings" },
    mutationFn: async (patch: Patch) => {
      const confirmed = queryClient.getQueryData<Settings>(settingsQueryKey);
      if (!confirmed) throw new Error("The settings are not loaded yet.");
      const body = { ...confirmed, ...patch };
      const { data, error } = await api().PUT("/api/v1/settings", { body });
      if (error) throw new Error(apiErrorMessage(error, "Could not save the settings."));
      return data;
    },
    onSuccess: async (saved) => {
      await queryClient.cancelQueries({ queryKey: settingsQueryKey });
      queryClient.setQueryData(settingsQueryKey, saved);
      void queryClient.invalidateQueries({ queryKey: watchListQueryKey("active") });
      void queryClient.invalidateQueries({ queryKey: watchListQueryKey("all") });
    },
  });
  return mutateAsync;
}
