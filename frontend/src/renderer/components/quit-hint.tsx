import { useEffect, useState } from "react";
import { bridge } from "@/lib/bridge";
import { isMac } from "@/lib/platform";

export const QUIT_HINT_LINGER_MS = 1200;

export function QuitHint() {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    let hideTimer: ReturnType<typeof setTimeout> | undefined;
    const off = bridge.quit.onShortcut((hint) => {
      clearTimeout(hideTimer);
      if (hint.state === "down") {
        setVisible(true);
        return;
      }
      hideTimer = setTimeout(() => setVisible(false), QUIT_HINT_LINGER_MS);
    });
    return () => {
      clearTimeout(hideTimer);
      off();
    };
  }, []);

  if (!visible) return null;
  const shortcut = isMac ? "⌘Q" : "Ctrl+Q";
  return (
    <div role="status" className="pointer-events-none fixed inset-x-0 top-[22%] z-100 flex justify-center">
      <div className="rounded-full bg-foreground/95 px-8 py-4 text-2xl font-semibold text-background shadow-xl">
        Hold {shortcut} or press it again to quit
      </div>
    </div>
  );
}
