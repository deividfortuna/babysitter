import { Children, Fragment, useId, type ReactNode } from "react";
import { BotIcon, CircleAlertIcon } from "lucide-react";
import type { PullRequest } from "@/hooks/usePulls";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Item, ItemActions, ItemContent, ItemGroup, ItemMedia, ItemTitle } from "@/components/ui/item";
import { avatarSource } from "@/lib/avatar";
import { initials } from "@/lib/initials";
import { cn } from "@/lib/utils";

const labelDots = [
  "bg-label-1",
  "bg-label-2",
  "bg-label-3",
  "bg-label-4",
  "bg-label-5",
  "bg-label-6",
  "bg-label-7",
  "bg-label-8",
];

const fnvOffset = 0x811c9dc5;
const fnvPrime = 0x01000193;

function labelDot(name: string): string {
  let hash = fnvOffset;
  for (const char of name) hash = Math.imul(hash ^ char.charCodeAt(0), fnvPrime);
  return labelDots[(hash >>> 0) % labelDots.length];
}

export function LabelBadges({ labels }: { labels: string[] }) {
  return (
    <span className="inline-flex flex-wrap gap-1.5">
      {labels.map((name) => (
        <Badge key={name} variant="outline" className="h-5 gap-1.5 font-normal text-muted-foreground">
          <span aria-hidden="true" className={cn("size-2 rounded-full", labelDot(name))} />
          {name}
        </Badge>
      ))}
    </span>
  );
}

const botSuffix = "[bot]";

type AuthorNameProps = { login: string; avatarUrl?: string };

export function AuthorName({ login, avatarUrl }: AuthorNameProps) {
  const bot = login.endsWith(botSuffix);
  const name = bot ? login.slice(0, -botSuffix.length) : login;
  const src = avatarSource(name, bot, avatarUrl);
  return (
    <span className="inline-flex items-center gap-1.5">
      <Avatar className="size-4.5">
        {src ? <AvatarImage src={src} alt="" /> : null}
        <AvatarFallback
          className={cn("text-3xs font-semibold", bot ? "bg-chart-1 text-white" : "bg-accent text-muted-foreground")}
        >
          {bot ? <BotIcon aria-hidden="true" className="size-2.5" /> : initials(name)}
        </AvatarFallback>
      </Avatar>
      {name}
    </span>
  );
}

export function DiffStat({ pull }: { pull?: PullRequest }) {
  if (!pull) return null;
  return (
    <span className="font-mono text-xs" title={`${pull.additions} lines added, ${pull.deletions} removed`}>
      <span className="text-success">+{pull.additions}</span>{" "}
      <span className="text-destructive">−{pull.deletions}</span>
    </span>
  );
}

export function PullsErrorAlert({ error }: { error: Error | null }) {
  if (!error) return null;
  return (
    <Alert variant="destructive" className="rounded-none border-x-0 border-t-0">
      <CircleAlertIcon />
      <AlertTitle>Labels and changed lines did not load</AlertTitle>
      <AlertDescription>{error.message}</AlertDescription>
    </Alert>
  );
}

function DotList({ children }: { children: ReactNode }) {
  return Children.toArray(children).map((part, index) => (
    <Fragment key={index}>
      {index > 0 ? <span aria-hidden="true">·</span> : null}
      {part}
    </Fragment>
  ));
}

type InboxGroupProps = {
  heading: ReactNode;
  empty?: ReactNode;
  children?: ReactNode;
};

export function InboxGroup({ heading, empty, children }: InboxGroupProps) {
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-1.5">
      <h2 id={headingId} className="flex py-1 text-body font-medium text-muted-foreground">
        {heading}
      </h2>
      {empty ? (
        <p className="py-3 text-body text-muted-foreground">{empty}</p>
      ) : (
        <ItemGroup className="gap-1.5">{children}</ItemGroup>
      )}
    </section>
  );
}

type RowContentProps = {
  icon: ReactNode;
  title: string;
  status: ReactNode;
  details: ReactNode;
  time: string;
  attention?: boolean;
  emphasized?: boolean;
};

const rowClassName = "w-full flex-nowrap items-start gap-3.5 rounded-xl px-3.5 py-3 text-left";

type CardLook = { attention?: boolean; clickable?: boolean };

function cardClassName({ attention = false, clickable = false }: CardLook): string {
  const attentionHover = attention && clickable;
  return cn(
    "rounded-xl bg-row ring-1 ring-border/60 transition-colors ring-inset",
    clickable && "hover:bg-accent/60",
    attention && "bg-attention/7 ring-attention/35",
    attentionHover && "hover:bg-attention/12",
  );
}

function RowContent({ icon, title, status, details, time, attention = false, emphasized = false }: RowContentProps) {
  const quiet = !attention && !emphasized;
  return (
    <>
      <ItemMedia className="pt-0.5 [&_svg]:size-4.5">{icon}</ItemMedia>
      <ItemContent className="min-w-0 gap-1.5">
        <div className="flex items-center gap-2.5">
          <ItemTitle className={cn("min-w-0 text-title", quiet && "text-foreground/75")}>
            <span className="truncate">{title}</span>
          </ItemTitle>
          <span className="ml-auto flex shrink-0 items-center gap-2">{status}</span>
        </div>
        <div className="flex flex-wrap items-center gap-2 text-body text-muted-foreground">
          <DotList>{details}</DotList>
          <span className="ml-auto">{time}</span>
        </div>
      </ItemContent>
    </>
  );
}

type InboxRowProps = RowContentProps & { onOpen: () => void; actions?: ReactNode };

export function InboxRow({ onOpen, actions, ...content }: InboxRowProps) {
  return (
    <div
      role="listitem"
      data-attention={content.attention || undefined}
      className={cardClassName({ attention: content.attention, clickable: true })}
    >
      <Item asChild className={rowClassName}>
        <button type="button" onClick={onOpen}>
          <RowContent {...content} />
        </button>
      </Item>
      {actions ? <div className="flex flex-wrap items-center gap-2 pr-3.5 pb-2.5 pl-11.5">{actions}</div> : null}
    </div>
  );
}

type InboxItemProps = RowContentProps & { actions: ReactNode };

export function InboxItem({ actions, ...content }: InboxItemProps) {
  return (
    <div
      role="listitem"
      data-attention={content.attention || undefined}
      className={cardClassName({ attention: content.attention })}
    >
      <Item className={rowClassName}>
        <RowContent {...content} />
        <ItemActions className="self-center">{actions}</ItemActions>
      </Item>
    </div>
  );
}
