import { useCallback, useState } from "react";
import { goBack, goForward, startHistory, visit, type View } from "@/lib/navigation";

export type HistoryControls = {
  canGoBack: boolean;
  canGoForward: boolean;
  onBack: () => void;
  onForward: () => void;
};

export function useViewHistory(initial: View) {
  const [history, setHistory] = useState(() => startHistory(initial));
  const navigate = useCallback((next: View) => setHistory((current) => visit(current, next)), []);
  const onBack = useCallback(() => setHistory(goBack), []);
  const onForward = useCallback(() => setHistory(goForward), []);
  const controls: HistoryControls = {
    canGoBack: history.back.length > 0,
    canGoForward: history.forward.length > 0,
    onBack,
    onForward,
  };
  return { view: history.current, navigate, controls };
}
