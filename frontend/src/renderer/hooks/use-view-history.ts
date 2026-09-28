import { useCallback, useState } from "react";
import { goBack, goForward, startHistory, visit, type View } from "@/lib/navigation";

export function useViewHistory(initial: View) {
  const [history, setHistory] = useState(() => startHistory(initial));
  const navigate = useCallback((next: View) => setHistory((current) => visit(current, next)), []);
  const back = useCallback(() => setHistory(goBack), []);
  const forward = useCallback(() => setHistory(goForward), []);
  return {
    view: history.current,
    navigate,
    back,
    forward,
    canGoBack: history.back.length > 0,
    canGoForward: history.forward.length > 0,
  };
}
