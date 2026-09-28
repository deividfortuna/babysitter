import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { pullListQueryKey } from "../lib/query-keys";
import { watchLabel } from "../lib/watch-status";

export type PullRequest = components["schemas"]["HttpdPullRequest"];

export function usePulls(enabled: boolean, state: "open" | "all" = "open") {
  return useQuery({
    queryKey: pullListQueryKey(state),
    enabled,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/prs", { params: { query: { state } } });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the pull requests."));
      return data.pullRequests;
    },
  });
}

export function usePullsByLabel(enabled: boolean, state: "open" | "all" = "open") {
  const pulls = usePulls(enabled, state);
  return useMemo(() => new Map((pulls.data ?? []).map((pr) => [watchLabel(pr), pr])), [pulls.data]);
}
