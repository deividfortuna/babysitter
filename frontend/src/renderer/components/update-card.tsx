import { useAppUpdate } from "@/hooks/useAppUpdate";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Spinner } from "@/components/ui/spinner";
import { releaseUrl, type UpdateStatus } from "../../shared/updates";

const SHOWN_STATES: UpdateStatus["state"][] = ["available", "downloading", "downloaded", "installing"];

function worthShowing(status: UpdateStatus | null): status is UpdateStatus {
  return status !== null && SHOWN_STATES.includes(status.state);
}

function title(status: UpdateStatus): string {
  const version = status.version ?? "";
  if (status.state === "available") return `Babysitter ${version} is out`;
  if (status.state === "downloading") return `Downloading ${version}`;
  return `Babysitter ${version} is ready`;
}

export function UpdateCard() {
  const { status, download, install } = useAppUpdate();
  if (!worthShowing(status)) return null;

  const percent = Math.floor(status.percent ?? 0);
  const installing = status.state === "installing";
  const restartable = status.state === "downloaded" || installing;
  const downloadFailed = status.state === "available" && status.message !== undefined;

  return (
    <Card role="region" aria-label="App update" className="gap-2.5 py-3">
      <CardHeader className="flex items-baseline justify-between gap-2 px-3">
        <CardTitle className="min-w-0 text-sm/5">{title(status)}</CardTitle>
        {status.state === "downloading" ? (
          <CardAction className="shrink-0 font-mono text-2xs/4 whitespace-nowrap text-muted-foreground">
            {percent}%
          </CardAction>
        ) : null}
        {status.state === "available" && status.version ? (
          <CardAction className="shrink-0 text-body/4">
            <a
              href={releaseUrl(status.version)}
              target="_blank"
              rel="noreferrer"
              className="text-muted-foreground underline-offset-2 hover:underline"
            >
              What's new
            </a>
          </CardAction>
        ) : null}
      </CardHeader>
      {status.state === "downloading" ? (
        <CardContent className="px-3">
          <Progress aria-label="Update downloaded" aria-valuenow={percent} value={percent} className="bg-muted" />
        </CardContent>
      ) : null}
      {restartable ? (
        <CardContent className="px-3">
          <p className="text-body/4.5 text-muted-foreground">
            {status.message ?? "The app restarts, and the watches continue."}
          </p>
        </CardContent>
      ) : null}
      {downloadFailed ? (
        <CardContent className="px-3">
          <p className="text-body/4.5 text-destructive">{status.message}</p>
        </CardContent>
      ) : null}
      {status.state === "available" ? (
        <CardFooter className="px-3">
          <Button variant="outline" size="sm" className="h-7 w-full text-body" onClick={() => void download()}>
            Download
          </Button>
        </CardFooter>
      ) : null}
      {restartable ? (
        <CardFooter className="px-3">
          <Button size="sm" className="h-7 w-full text-body" disabled={installing} onClick={() => void install()}>
            {installing ? <Spinner data-icon="inline-start" /> : null}
            {installing ? "Restarting…" : "Restart to update"}
          </Button>
        </CardFooter>
      ) : null}
    </Card>
  );
}
