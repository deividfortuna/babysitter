import { useState } from "react";
import { CircleAlertIcon } from "lucide-react";
import { useMergeWatch, type MergeMethod, type Watch } from "@/hooks/useWatches";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { MergeMethodSelect } from "@/components/merge-method-select";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { mergeMethodText, watchLabel } from "@/lib/watch-status";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: Watch;
  onMerged: (watch: Watch) => void;
};

export function MergeWatchDialog({ open, onOpenChange, watch, onMerged }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-(--size-dialog) sm:max-w-(--size-dialog-max)">
        <MergeWatchForm
          watch={watch}
          onMerged={(merged) => {
            onOpenChange(false);
            onMerged(merged);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

function MergeWatchForm({ watch, onMerged }: { watch: Watch; onMerged: (watch: Watch) => void }) {
  const [method, setMethod] = useState<MergeMethod>(watch.mergeMethod ?? "");
  const merge = useMergeWatch();

  const submit = () => {
    merge.mutate({ id: watch.id, method }, { onSuccess: onMerged });
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>Merge {watchLabel(watch)}</DialogTitle>
        <DialogDescription>
          The pull request is merged into {watch.baseRef} and the watch stops. The daemon looks at the pull request once
          more first and refuses if anything changed.
        </DialogDescription>
      </DialogHeader>

      <Field>
        <FieldLabel htmlFor="merge-method" className="eyebrow">
          Merge method
        </FieldLabel>
        <MergeMethodSelect id="merge-method" className="w-full" value={method} onChange={setMethod} />
        <FieldDescription>
          {method === ""
            ? "Squash when the repository allows it, else a merge commit, else a rebase."
            : `The pull request is merged with ${mergeMethodText(method)}.`}
        </FieldDescription>
      </Field>

      {merge.error ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{merge.error.message}</AlertTitle>
        </Alert>
      ) : null}

      <DialogFooter className="sm:justify-start">
        <Button type="button" onClick={submit} disabled={merge.isPending}>
          {merge.isPending ? <Spinner data-icon="inline-start" /> : null}
          Merge
        </Button>
        <DialogClose asChild>
          <Button type="button" variant="ghost">
            Cancel
          </Button>
        </DialogClose>
      </DialogFooter>
    </>
  );
}
