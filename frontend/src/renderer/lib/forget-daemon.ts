import type { QueryClient } from "@tanstack/react-query";
import { appVersionQueryKey, discoveryQueryKey, settingsMutationKey } from "./query-keys";

const APP_QUERIES = new Set<string>([discoveryQueryKey[0], appVersionQueryKey[0]]);

export function forgetDaemon(queryClient: QueryClient): void {
  queryClient.removeQueries({ predicate: (query) => !APP_QUERIES.has(String(query.queryKey[0])) });
  const mutations = queryClient.getMutationCache();
  for (const saving of mutations.findAll({ mutationKey: settingsMutationKey })) mutations.remove(saving);
}
