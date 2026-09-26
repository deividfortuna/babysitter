import type { ComponentProps } from "react";
import type { ApprovalMode } from "@/hooks/useProposals";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type Props = Omit<ComponentProps<typeof SelectTrigger>, "value" | "onChange"> & {
  value: ApprovalMode;
  onChange: (value: ApprovalMode) => void;
};

const MODES: ApprovalMode[] = ["manual", "auto"];

export function ApprovalModeSelect({ value, onChange, disabled, ...trigger }: Props) {
  return (
    <Select value={value} disabled={disabled} onValueChange={(next) => onChange(next as ApprovalMode)}>
      <SelectTrigger {...trigger}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {MODES.map((mode) => (
          <SelectItem key={mode} value={mode}>
            {mode}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
