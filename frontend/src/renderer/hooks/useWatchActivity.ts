import { useQuery } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { watchActivityQueryKey } from "../lib/query-keys";

export type Activity = components["schemas"]["HttpdActivity"];

export function useWatchActivity(id: number | null) {
  return useQuery({
    queryKey: watchActivityQueryKey(id ?? 0),
    enabled: id !== null,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/watches/{id}/activity", {
        params: { path: { id: id ?? 0 }, query: { since: 0, limit: 200 } },
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the activity."));
      return data.activity;
    },
  });
}
