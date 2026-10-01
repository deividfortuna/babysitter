import iconDark from "../../../assets/icon-dark.svg";
import iconLight from "../../../assets/icon.svg";
import { cn } from "@/lib/utils";

export function AppIcon({ className }: { className?: string }) {
  return (
    <>
      <img src={iconLight} alt="" aria-hidden className={cn("shrink-0 dark:hidden", className)} />
      <img src={iconDark} alt="" aria-hidden className={cn("hidden shrink-0 dark:block", className)} />
    </>
  );
}
