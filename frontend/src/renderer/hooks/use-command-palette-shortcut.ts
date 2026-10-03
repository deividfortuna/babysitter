import { useCommandShortcut } from "@/hooks/use-command-shortcut";

export function useCommandPaletteShortcut(onOpen: () => void, allowed: boolean) {
  useCommandShortcut("p", onOpen, allowed);
}
