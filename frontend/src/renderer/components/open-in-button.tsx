import { useCallback } from "react";
import { ChevronDownIcon } from "lucide-react";
import { watchFolder, type OpenTarget } from "../../shared/open-in";
import { useCommandShortcut } from "@/hooks/use-command-shortcut";
import { useOpenIn, type OpenFolder } from "@/hooks/use-open-in";
import type { Watch } from "@/hooks/useWatches";
import { EditorLogo, openTargetLabel } from "@/components/editor-logo";
import { Tip, TipLabel } from "@/components/tip";
import { ViewHeaderButton } from "@/components/view-header";
import { Button } from "@/components/ui/button";
import { ButtonGroup } from "@/components/ui/button-group";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { openInShortcut } from "@/lib/shortcuts";

type Props = {
  watch: Watch;
  onThisMachine: boolean;
  onOpen: (request: OpenFolder) => void;
};

export function OpenInButton({ watch, onThisMachine, onOpen }: Props) {
  const hasFolder = onThisMachine && watchFolder(watch) !== null;
  const { targets, preferred, setPreferred } = useOpenIn(hasFolder);
  const open = useCallback(
    (target: OpenTarget) => {
      setPreferred(target);
      onOpen({ watchId: watch.id, target });
    },
    [setPreferred, onOpen, watch.id],
  );
  const openPreferred = useCallback(() => {
    if (preferred) open(preferred);
  }, [open, preferred]);
  const shown = hasFolder && preferred !== null;
  useCommandShortcut("o", openPreferred, shown);
  if (!shown) return null;

  const shortcut = openInShortcut();

  return (
    <ButtonGroup>
      <Tip label={<TipLabel label={`Open in ${openTargetLabel(preferred)}`} shortcut={shortcut} />}>
        <ViewHeaderButton variant="outline" onClick={openPreferred}>
          <EditorLogo target={preferred} className="size-3.5" />
          Open
        </ViewHeaderButton>
      </Tip>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Tip label="Open in…">
            <Button variant="outline" size="icon-xs" aria-label="Open in…">
              <ChevronDownIcon />
            </Button>
          </Tip>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-48">
          {targets.map((target) => (
            <DropdownMenuItem key={target} className="text-body" onClick={() => open(target)}>
              <EditorLogo target={target} />
              {openTargetLabel(target)}
              {target === preferred ? <DropdownMenuShortcut>{shortcut}</DropdownMenuShortcut> : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </ButtonGroup>
  );
}
