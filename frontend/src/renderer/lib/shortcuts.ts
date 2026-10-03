import { isMac } from "@/lib/platform";

export type Modifier = "metaKey" | "ctrlKey" | "altKey";

type ModifierKey = Modifier | "shiftKey";

const MODIFIER_KEYS: ModifierKey[] = ["metaKey", "ctrlKey", "altKey", "shiftKey"];

export type ShortcutKey = Pick<KeyboardEvent, "code" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">;

export function onlyModifiers(event: ShortcutKey, modifiers: ModifierKey[]): boolean {
  return MODIFIER_KEYS.every((modifier) => event[modifier] === modifiers.includes(modifier));
}

export function onlyModifier(event: ShortcutKey, modifier: Modifier): boolean {
  return onlyModifiers(event, [modifier]);
}

export function commandModifier(): Modifier {
  return isMac ? "metaKey" : "ctrlKey";
}

function commandShortcut(letter: string): string {
  return isMac ? `⌘${letter}` : `Ctrl+${letter}`;
}

function commandAltShortcut(letter: string): string {
  return isMac ? `⌥⌘${letter}` : `Ctrl+Alt+${letter}`;
}

export function sidebarShortcut(): string {
  return commandShortcut("B");
}

export function panelShortcut(): string {
  return commandAltShortcut("B");
}

export function terminalShortcut(): string {
  return isMac ? "⌃`" : "Ctrl+`";
}

export function openInShortcut(): string {
  return commandShortcut("O");
}

export const REFRESH_KEY = "r";
export const REMOVE_KEY = "d";
