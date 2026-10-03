import { isMac } from "@/lib/platform";

export type Modifier = "metaKey" | "ctrlKey" | "altKey";

export type ShortcutKey = Pick<KeyboardEvent, "code" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">;

export function onlyModifier(event: ShortcutKey, modifier: Modifier): boolean {
  const pressed = [event.metaKey, event.ctrlKey, event.altKey, event.shiftKey].filter(Boolean).length;
  return event[modifier] && pressed === 1;
}

export function commandModifier(): Modifier {
  return isMac ? "metaKey" : "ctrlKey";
}

function commandShortcut(letter: string): string {
  return isMac ? `⌘${letter}` : `Ctrl+${letter}`;
}

export function sidebarShortcut(): string {
  return commandShortcut("B");
}

export function openInShortcut(): string {
  return commandShortcut("O");
}
