import { pressedKey } from "@/hooks/use-command-shortcut";
import { onlyModifiers } from "@/lib/shortcuts";

export function pressesPlainKey(event: KeyboardEvent, key: string): boolean {
  if (event.isComposing || event.repeat) return false;
  return pressedKey(event) === key && onlyModifiers(event, []);
}
