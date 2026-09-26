import claudeLogo from "../../../assets/agents/claude.svg";
import copilotLogo from "../../../assets/agents/copilot.svg";
import type { Provider } from "@/hooks/useProviders";
import { cn } from "@/lib/utils";

const LOGOS: Record<Provider["id"], string> = {
  claude: claudeLogo,
  copilot: copilotLogo,
};

export function AgentLogo({ provider, className }: { provider: Provider["id"]; className?: string }) {
  return <img src={LOGOS[provider]} alt="" aria-hidden className={cn("size-4 shrink-0", className)} />;
}
