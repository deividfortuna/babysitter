import { useState } from "react";

export function useCopy(resetAfterMs = 1_500) {
  const [copied, setCopied] = useState(false);

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), resetAfterMs);
    } catch {}
  }

  return { copied, copy };
}
