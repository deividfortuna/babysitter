import { EyeIcon } from "lucide-react";
import { Meta } from "@/components/status-badges";
import { Button } from "@/components/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";

type Props = {
  onWatchPR: () => void;
  onAddRepo: () => void;
};

export function FirstRun({ onWatchPR, onAddRepo }: Props) {
  return (
    <Empty className="flex-1 py-16">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <EyeIcon />
        </EmptyMedia>
        <EmptyTitle className="text-xl">No pull request is watched</EmptyTitle>
        <EmptyDescription className="max-w-sm text-sm">
          Hand one over and go do something else. An agent works on each review comment and failed check, and the app
          tells you when it needs you.
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <div className="flex gap-2">
          <Button onClick={onWatchPR}>Watch a pull request</Button>
          <Button variant="outline" onClick={onAddRepo}>
            Add a repository
          </Button>
        </div>
        <Meta className="mt-2 border-t pt-3">or, in a checkout: babysitter watch start</Meta>
      </EmptyContent>
    </Empty>
  );
}
