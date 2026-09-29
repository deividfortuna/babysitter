import { useQuery } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { providersQueryKey } from "../lib/query-keys";

export type Provider = components["schemas"]["HttpdProvider"];
export type ProviderModel = components["schemas"]["HttpdProviderModel"];
export type ProviderEffort = components["schemas"]["HttpdProviderEffort"];

export function useProviders(enabled: boolean) {
  return useQuery({
    queryKey: providersQueryKey,
    enabled,
    staleTime: Infinity,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/providers");
      if (error) throw new Error(apiErrorMessage(error, "Could not load the AI providers."));
      return data.providers ?? [];
    },
  });
}
