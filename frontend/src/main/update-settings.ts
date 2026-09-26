import { mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import path from "node:path";
import { parseSettingsPatch, type UpdateSettings } from "../shared/updates";

const FILE = "update-settings.json";

export function readUpdateSettings(dir: string): Partial<UpdateSettings> {
  try {
    return parseSettingsPatch(JSON.parse(readFileSync(path.join(dir, FILE), "utf8")));
  } catch {
    return {};
  }
}

export function writeUpdateSettings(dir: string, settings: Partial<UpdateSettings>): void {
  const file = path.join(dir, FILE);
  const temporary = `${file}.tmp`;
  try {
    mkdirSync(dir, { recursive: true });
    writeFileSync(temporary, JSON.stringify(settings));
    renameSync(temporary, file);
  } catch {}
}
