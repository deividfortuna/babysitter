import { useQuery } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { rateLimitQueryKey } from "../lib/query-keys";

export type RateLimit = components["schemas"]["HttpdRateLimit"];

const REFRESH_MS = 30_000;

export function useRateLimit(enabled: boolean) {
  return useQuery({
    queryKey: rateLimitQueryKey,
    enabled,
    refetchInterval: REFRESH_MS,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/ratelimit");
      if (error) throw new Error(apiErrorMessage(error, "Could not read the GitHub rate limit."));
      return data;
    },
  });
}
