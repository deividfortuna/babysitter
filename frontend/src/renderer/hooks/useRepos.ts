import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { pullsQueryKey, reposQueryKey } from "../lib/query-keys";

export type Repo = components["schemas"]["HttpdRepo"];

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
