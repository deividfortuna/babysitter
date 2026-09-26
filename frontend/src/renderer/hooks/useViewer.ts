import { useQuery } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { viewerQueryKey } from "../lib/query-keys";

export type Viewer = components["schemas"]["HttpdViewer"];

export function useViewer(enabled: boolean) {
  return useQuery({
    queryKey: viewerQueryKey,
    enabled,
    staleTime: Infinity,
    retry: false,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/viewer");
      if (error) throw new Error(apiErrorMessage(error, "Could not read the GitHub account."));
      return data;
    },
  });
}
