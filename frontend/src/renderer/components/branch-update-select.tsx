import type { ComponentProps } from "react";
import type { BranchUpdate } from "@/hooks/useWatches";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type Props = Omit<ComponentProps<typeof SelectTrigger>, "value" | "onChange"> & {
  value: BranchUpdate;
  onChange: (value: BranchUpdate) => void;
};

export const BRANCH_UPDATES: { value: BranchUpdate; label: string }[] = [
  { value: "rebase", label: "Rebase" },
  { value: "merge", label: "Merge" },
];

export function branchUpdateLabel(value: BranchUpdate): string {
  return BRANCH_UPDATES.find((update) => update.value === value)?.label ?? value;
}

export function BranchUpdateSelect({ value, onChange, disabled, ...trigger }: Props) {
  return (
    <Select value={value} disabled={disabled} onValueChange={(next) => onChange(next as BranchUpdate)}>
      <SelectTrigger {...trigger}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {BRANCH_UPDATES.map((update) => (
          <SelectItem key={update.value} value={update.value}>
            {update.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
