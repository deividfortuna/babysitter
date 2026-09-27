import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { watchListQueryKey, watchesQueryKey } from "../lib/query-keys";

export type Watch = components["schemas"]["HttpdWatch"];
export type WatchSummary = components["schemas"]["HttpdWatchSummary"];
export type StartWatchRequest = components["schemas"]["HttpdStartWatchRequest"];
export type MergeMethod = NonNullable<Watch["mergeMethod"]>;

export function useWatches(enabled: boolean, status: "active" | "all" = "active") {
  return useQuery({
    queryKey: watchListQueryKey(status),
    enabled,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/watches", { params: { query: { status } } });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the watched pull requests."));
      return data.watches ?? [];
    },
  });
}

export function useStartWatch() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: StartWatchRequest) => {
      const { data, error } = await api().POST("/api/v1/watches", { body });
      if (error) throw new Error(apiErrorMessage(error, "Could not start the watch."));
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: watchesQueryKey });
    },
  });
}

export function useStopWatch() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, keepWorktree }: { id: number; keepWorktree?: boolean }) => {
      const { data, error } = await api().POST("/api/v1/watches/{id}/stop", {
        params: { path: { id } },
        body: { keepWorktree },
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not stop the watch."));
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: watchesQueryKey });
    },
  });
}

export function useMergeWatch() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, method = "", approve }: { id: number; method?: MergeMethod; approve?: boolean }) => {
      const { data, error } = await api().POST("/api/v1/watches/{id}/merge", {
        params: { path: { id } },
        body: approve ? { method, approve } : { method },
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not merge the pull request."));
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: watchesQueryKey });
    },
  });
}

export function usePollWatch() {
  return useMutation({
    mutationFn: async (id: number) => {
      const { error } = await api().POST("/api/v1/watches/{id}/poll", { params: { path: { id } } });
      if (error) throw new Error(apiErrorMessage(error, "Could not poll the watch."));
    },
  });
}

export function useWatch(id: number | null) {
  return useQuery({
    queryKey: ["watches", "one", id ?? 0] as const,
    enabled: id !== null,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/watches/{id}", { params: { path: { id: id ?? 0 } } });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the watch."));
      return data;
    },
  });
}

type WatchUpdate = components["schemas"]["UpdateWatchParams"];

export function useUpdateWatch() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, ...body }: WatchUpdate & { id: number }) => {
      const { data, error } = await api().PATCH("/api/v1/watches/{id}", { params: { path: { id } }, body });
      if (error) throw new Error(apiErrorMessage(error, "Could not change the watch settings."));
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: watchesQueryKey });
    },
  });
}
