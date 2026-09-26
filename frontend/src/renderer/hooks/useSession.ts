import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiErrorMessage } from "../lib/api-client";
import { watchOutputQueryKey, watchesQueryKey } from "../lib/query-keys";

export function useWatchOutput(id: number | null, lines = 200) {
  return useQuery({
    queryKey: watchOutputQueryKey(id ?? 0),
    enabled: id !== null,
    refetchInterval: 5_000,
    queryFn: async () => {
      const { data, error } = await api().GET("/api/v1/watches/{id}/output", {
        params: { path: { id: id ?? 0 }, query: { lines } },
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the output of the agent."));
      return data.output;
    },
  });
}

export function useSendMessage() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, message }: { id: number; message: string }) => {
      const { data, error } = await api().POST("/api/v1/watches/{id}/send", {
        params: { path: { id } },
        body: { message },
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not send the message."));
      return data;
    },
    onSuccess: (_, { id }) => {
      void queryClient.invalidateQueries({ queryKey: watchesQueryKey });
      void queryClient.invalidateQueries({ queryKey: watchOutputQueryKey(id) });
    },
  });
}
