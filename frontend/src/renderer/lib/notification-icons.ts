import { CircleAlertIcon, GitMergeIcon, MessageSquareIcon, PlayIcon, SparklesIcon } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { NotificationKind } from "../../shared/notifications";

export const KIND_ICON: Record<NotificationKind, LucideIcon> = {
  agent: SparklesIcon,
  review: MessageSquareIcon,
  checks: CircleAlertIcon,
  watch: PlayIcon,
  merge: GitMergeIcon,
};
