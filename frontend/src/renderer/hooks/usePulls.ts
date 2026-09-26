import { useQuery } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { pullsQueryKey } from "../lib/query-keys";

export type PullRequest = components["schemas"]["HttpdPullRequest"];

export function usePulls(enabled: boolean) {
  return useQuery({
    queryKey: pullsQueryKey,
    enabled,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/prs");
      if (error) throw new Error(apiErrorMessage(error, "Could not load the pull requests."));
      return data.pullRequests;
    },
  });
}
