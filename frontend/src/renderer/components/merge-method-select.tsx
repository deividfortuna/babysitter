import type { ComponentProps } from "react";
import type { MergeMethod } from "@/hooks/useWatches";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { fromSelectValue, toSelectValue } from "@/lib/select-value";

type Props = Omit<ComponentProps<typeof SelectTrigger>, "value" | "onChange"> & {
  value: MergeMethod;
  onChange: (value: MergeMethod) => void;
};

const METHODS: { value: MergeMethod; label: string }[] = [
  { value: "", label: "Repository default" },
  { value: "squash", label: "Squash" },
  { value: "merge", label: "Merge commit" },
  { value: "rebase", label: "Rebase" },
];

export function mergeMethodLabel(value: MergeMethod): string {
  return METHODS.find((method) => method.value === value)?.label ?? value;
}

export function MergeMethodSelect({ value, onChange, disabled, ...trigger }: Props) {
  return (
    <Select
      value={toSelectValue(value)}
      disabled={disabled}
      onValueChange={(next) => onChange(fromSelectValue(next) as MergeMethod)}
    >
      <SelectTrigger {...trigger}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {METHODS.map((method) => (
          <SelectItem key={method.value} value={toSelectValue(method.value)}>
            {method.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
