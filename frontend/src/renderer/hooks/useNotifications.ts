import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { api, apiErrorMessage } from "../lib/api-client";
import { notificationsQueryKey } from "../lib/query-keys";

export type Notification = components["schemas"]["HttpdNotification"];

const PAGE_SIZE = 200;

export function useNotifications(enabled = true) {
  return useQuery({
    queryKey: notificationsQueryKey,
    enabled,
    staleTime: Infinity,
    refetchInterval: false,
    queryFn: async (): Promise<{ notifications: Notification[]; unreadCount: number }> => {
      const { data, error } = await api().GET("/api/v1/notifications", {
        params: { query: { limit: String(PAGE_SIZE) } },
      });
      if (error) throw new Error(apiErrorMessage(error, "Could not load the notifications."));
      return { notifications: data.notifications ?? [], unreadCount: data.unreadCount };
    },
  });
}

export function useReadNotifications() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (ids?: number[]) => {
      const { data, error } = await api().POST("/api/v1/notifications/read", { body: ids?.length ? { ids } : {} });
      if (error) throw new Error(apiErrorMessage(error, "Could not mark the notifications as seen."));
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: notificationsQueryKey });
    },
  });
}
