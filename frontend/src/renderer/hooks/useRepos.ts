import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { pullsQueryKey, repoConfigQueryKey, repoQueueQueryKey, reposQueryKey } from "../lib/query-keys";

export type Repo = components["schemas"]["HttpdRepo"];
export type RepoConfig = components["schemas"]["HttpdRepoConfig"];
export type RepoConfigUpdate = components["schemas"]["RepoConfigParams"];
export type WatchOverrides = components["schemas"]["HttpdWatchOverrides"];
export type QueuedPullRequest = components["schemas"]["HttpdQueuedPullRequest"];
export type DependabotScope = RepoConfig["dependabotScope"];
export type DependabotApproval = RepoConfig["dependabotApproval"];

export function useRepos(enabled: boolean) {
  return useQuery({
    queryKey: reposQueryKey,
    enabled,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/repos");
      if (error) throw new Error(apiErrorMessage(error, "Could not load the repositories."));
      return data.repos;
    },
  });
}

export function useAddRepo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (fullName: string) => {
      const { data, error } = await api().POST("/api/v1/repos", { body: { fullName } });
      if (error) throw new Error(apiErrorMessage(error, "Could not add the repository."));
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: reposQueryKey });
    },
  });
}

export function useRemoveRepo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => {
      const { error } = await api().DELETE("/api/v1/repos/{id}", { params: { path: { id } } });
      if (error) throw new Error(apiErrorMessage(error, "Could not remove the repository."));
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: reposQueryKey });
      void queryClient.invalidateQueries({ queryKey: pullsQueryKey });
    },
  });
}

export function useRequestSync() {
  return useMutation({
    mutationFn: async () => {
      const { error } = await api().POST("/api/v1/sync");
      if (error) throw new Error(apiErrorMessage(error, "Could not request a sync."));
    },
  });
}

export function useRepoConfig(repoId: number | null) {
  return useQuery({
    queryKey: repoConfigQueryKey(repoId ?? 0),
    enabled: repoId !== null,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/repos/{id}/config", {
        params: { path: { id: repoId ?? 0 } },
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the repository settings."));
      return data;
    },
  });
}

export function useUpdateRepoConfig(repoId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: RepoConfigUpdate) => {
      const { data, error } = await api().PATCH("/api/v1/repos/{id}/config", {
        params: { path: { id: repoId } },
        body,
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not change the repository settings."));
      return data;
    },
    onSuccess: (config) => {
      queryClient.setQueryData(repoConfigQueryKey(repoId), config);
      void queryClient.invalidateQueries({ queryKey: repoQueueQueryKey(repoId) });
    },
  });
}

export function useRepoQueue(repoId: number | null) {
  return useQuery({
    queryKey: repoQueueQueryKey(repoId ?? 0),
    enabled: repoId !== null,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/repos/{id}/queue", { params: { path: { id: repoId ?? 0 } } });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the queue of the repository."));
      return data.pullRequests ?? [];
    },
  });
}
