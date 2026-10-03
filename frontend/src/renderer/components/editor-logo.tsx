import { FolderIcon } from "lucide-react";
import antigravityLogo from "../../../assets/editors/antigravity.svg";
import aquaLogo from "../../../assets/editors/aqua.svg";
import clionLogo from "../../../assets/editors/clion.svg";
import cursorLogo from "../../../assets/editors/cursor.svg";
import datagripLogo from "../../../assets/editors/datagrip.svg";
import dataspellLogo from "../../../assets/editors/dataspell.svg";
import fileExplorerLogo from "../../../assets/editors/file-explorer.svg";
import finderLogo from "../../../assets/editors/finder.svg";
import golandLogo from "../../../assets/editors/goland.svg";
import ideaLogo from "../../../assets/editors/idea.svg";
import kiroLogo from "../../../assets/editors/kiro.svg";
import phpstormLogo from "../../../assets/editors/phpstorm.svg";
import pycharmLogo from "../../../assets/editors/pycharm.svg";
import riderLogo from "../../../assets/editors/rider.svg";
import rubymineLogo from "../../../assets/editors/rubymine.svg";
import rustroverLogo from "../../../assets/editors/rustrover.svg";
import traeLogo from "../../../assets/editors/trae.svg";
import vscodeInsidersLogo from "../../../assets/editors/vscode-insiders.svg";
import vscodeLogo from "../../../assets/editors/vscode.svg";
import vscodiumLogo from "../../../assets/editors/vscodium.svg";
import webstormLogo from "../../../assets/editors/webstorm.svg";
import zedLogo from "../../../assets/editors/zed.svg";
import { FILE_MANAGER, findEditor, type EditorId, type OpenTarget } from "../../shared/open-in";
import { isMac, isWindows } from "@/lib/platform";
import { cn } from "@/lib/utils";

const LOGOS: Record<EditorId, string> = {
  cursor: cursorLogo,
  trae: traeLogo,
  kiro: kiroLogo,
  vscode: vscodeLogo,
  "vscode-insiders": vscodeInsidersLogo,
  vscodium: vscodiumLogo,
  zed: zedLogo,
  antigravity: antigravityLogo,
  idea: ideaLogo,
  aqua: aquaLogo,
  clion: clionLogo,
  datagrip: datagripLogo,
  dataspell: dataspellLogo,
  goland: golandLogo,
  phpstorm: phpstormLogo,
  pycharm: pycharmLogo,
  rider: riderLogo,
  rubymine: rubymineLogo,
  rustrover: rustroverLogo,
  webstorm: webstormLogo,
};

type FileManagerApp = { label: string; logo: string | null };

function systemFileManager(): FileManagerApp {
  if (isMac) return { label: "Finder", logo: finderLogo };
  if (isWindows) return { label: "File Explorer", logo: fileExplorerLogo };
  return { label: "Files", logo: null };
}

const FILE_MANAGER_APP = systemFileManager();

export function openTargetLabel(target: OpenTarget): string {
  return target === FILE_MANAGER ? FILE_MANAGER_APP.label : (findEditor(target)?.label ?? target);
}

export function EditorLogo({ target, className }: { target: OpenTarget; className?: string }) {
  const src = target === FILE_MANAGER ? FILE_MANAGER_APP.logo : LOGOS[target];
  if (!src) return <FolderIcon aria-hidden className={cn("size-4 shrink-0 text-muted-foreground", className)} />;
  return <img src={src} alt="" aria-hidden className={cn("size-4 shrink-0", className)} />;
}
