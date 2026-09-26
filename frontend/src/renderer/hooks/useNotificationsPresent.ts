import { useEffect, useState } from "react";
import { bridge } from "@/lib/bridge";

export function useNotificationsPresent(): boolean | null {
  const [present, setPresent] = useState<boolean | null>(null);
  useEffect(() => {
    let live = true;
    void bridge.notifications
      .supported()
      .catch(() => false)
      .then((supported) => {
        if (live) setPresent(supported);
      });
    return () => {
      live = false;
    };
  }, []);
  return present;
}
