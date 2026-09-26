import { SidebarTrigger } from "@/components/ui/sidebar";
import { isMac } from "@/lib/platform";

export function AppHeader() {
  if (isMac) return null;
  return (
    <header className="flex h-10 shrink-0 items-center px-3">
      <SidebarTrigger />
    </header>
  );
}

export function TitlebarSidebarTrigger() {
  if (!isMac) return null;
  return <SidebarTrigger className="fixed top-[calc((var(--titlebar-height)-(--spacing(7)))/2)] left-20 z-20" />;
}
