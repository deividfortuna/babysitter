import { useState, type SubmitEvent } from "react";
import { PlusIcon } from "lucide-react";
import { useAddRepo } from "@/hooks/useRepos";
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
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  enabled: boolean;
};

export function AddRepoDialog({ open, onOpenChange, enabled }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-(--size-dialog) sm:max-w-(--size-dialog-max)">
        <AddRepoForm enabled={enabled} onAdded={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

function AddRepoForm({ enabled, onAdded }: { enabled: boolean; onAdded: () => void }) {
  const addRepo = useAddRepo();
  const [fullName, setFullName] = useState("");

  function onSubmit(event: SubmitEvent) {
    event.preventDefault();
    const value = fullName.trim();
    if (!value) return;
    addRepo.mutate(value, { onSuccess: onAdded });
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>Add a repository</DialogTitle>
        <DialogDescription>The daemon syncs its open pull requests so you can pick one to watch.</DialogDescription>
      </DialogHeader>
      <FieldGroup>
        <Field data-invalid={addRepo.isError || undefined}>
          <FieldLabel htmlFor="repo-full-name" className="eyebrow">
            Repository
          </FieldLabel>
          <Input
            id="repo-full-name"
            placeholder="owner/name"
            autoComplete="off"
            value={fullName}
            disabled={!enabled}
            aria-invalid={addRepo.isError || undefined}
            onChange={(e) => setFullName(e.target.value)}
          />
          {addRepo.error ? (
            <FieldError>{addRepo.error.message}</FieldError>
          ) : (
            <FieldDescription>
              Your token needs read access to it. Push access is checked when you start a watch.
            </FieldDescription>
          )}
        </Field>
      </FieldGroup>
      <DialogFooter className="sm:justify-start">
        <Button type="submit" disabled={!enabled || !fullName.trim() || addRepo.isPending}>
          {addRepo.isPending ? <Spinner data-icon="inline-start" /> : <PlusIcon data-icon="inline-start" />}
          Add
        </Button>
        <DialogClose asChild>
          <Button type="button" variant="ghost">
            Cancel
          </Button>
        </DialogClose>
      </DialogFooter>
    </form>
  );
}
