import { watchFolder, type OpenTarget } from "../../shared/open-in";
import { useOpenIn, type OpenFolder } from "@/hooks/use-open-in";
import type { Watch } from "@/hooks/useWatches";
import { EditorLogo, openTargetLabel } from "@/components/editor-logo";
import { SplitButton } from "@/components/split-button";
import { Tip } from "@/components/tip";
import { ViewHeaderButton } from "@/components/view-header";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";

type Props = {
  watch: Watch;
  onThisMachine: boolean;
  onOpen: (request: OpenFolder) => void;
};

export function OpenInButton({ watch, onThisMachine, onOpen }: Props) {
  const hasFolder = onThisMachine && watchFolder(watch) !== null;
  const { targets, preferred, setPreferred } = useOpenIn(hasFolder);
  if (!hasFolder) return null;
  if (!preferred) return null;

  const open = (target: OpenTarget) => {
    setPreferred(target);
    onOpen({ watchId: watch.id, target });
  };

  const items = targets.map((target) => (
    <DropdownMenuItem key={target} onClick={() => open(target)}>
      <EditorLogo target={target} />
      {openTargetLabel(target)}
    </DropdownMenuItem>
  ));

  return (
    <SplitButton variant="outline" size="xs" label="Open in…" more={items}>
      <Tip label={`Open in ${openTargetLabel(preferred)}`}>
        <ViewHeaderButton variant="outline" onClick={() => open(preferred)}>
          <EditorLogo target={preferred} className="size-3.5" />
          Open
        </ViewHeaderButton>
      </Tip>
    </SplitButton>
  );
}
