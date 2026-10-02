import { useState, type SubmitEvent } from "react";
import { PlugZapIcon } from "lucide-react";
import type { DiscoveredDaemon } from "../../shared/connections";
import { usePairConnection } from "@/hooks/useConnections";
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
  found?: DiscoveredDaemon | null;
};

export function PairDialog({ open, onOpenChange, found }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-(--size-dialog) sm:max-w-(--size-dialog-max)">
        <PairForm key={found?.url ?? "manual"} found={found ?? null} onPaired={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

function PairForm({ found, onPaired }: { found: DiscoveredDaemon | null; onPaired: () => void }) {
  const pair = usePairConnection();
  const [link, setLink] = useState(found?.url ?? "");
  const [token, setToken] = useState("");

  function onSubmit(event: SubmitEvent) {
    event.preventDefault();
    if (!link.trim()) return;
    pair.mutate({ link, token, name: found?.name }, { onSuccess: onPaired });
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>{found ? `Connect to ${found.name}` : "Connect to a remote daemon"}</DialogTitle>
        <DialogDescription>
          {found
            ? `Found on your network at ${found.address}:${found.port}. The app shows its watches, and the daemon of this computer stops.`
            : "Show the watches of a daemon that runs on another machine. The daemon of this computer stops while you look at it."}
        </DialogDescription>
      </DialogHeader>
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="pair-link" className="eyebrow">
            Pairing link or address
          </FieldLabel>
          <Input
            id="pair-link"
            placeholder="http://studio.local:7420/#token=…"
            autoComplete="off"
            spellCheck={false}
            value={link}
            onChange={(e) => setLink(e.target.value)}
          />
          <FieldDescription>
            On that machine, run <code className="font-mono text-2xs">babysitter daemon pair</code> and paste one of the
            links it prints.
          </FieldDescription>
        </Field>
        <Field data-invalid={pair.isError || undefined}>
          <FieldLabel htmlFor="pair-token" className="eyebrow">
            Token
          </FieldLabel>
          <Input
            id="pair-token"
            type="password"
            placeholder={found ? "Paste the token" : "Not needed when the link holds it"}
            autoComplete="off"
            value={token}
            aria-invalid={pair.isError || undefined}
            onChange={(e) => setToken(e.target.value)}
          />
          {pair.error ? (
            <FieldError>{pair.error.message}</FieldError>
          ) : (
            <FieldDescription>Whoever has the token can read and drive every watch of that daemon.</FieldDescription>
          )}
        </Field>
      </FieldGroup>
      <DialogFooter className="sm:justify-start">
        <Button type="submit" disabled={!link.trim() || pair.isPending}>
          {pair.isPending ? <Spinner data-icon="inline-start" /> : <PlugZapIcon data-icon="inline-start" />}
          Connect
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
