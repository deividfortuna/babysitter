import type { ComponentProps } from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { fromSelectValue, toSelectValue } from "@/lib/select-value";

export type Option<T extends string> = { value: T; label: string };

type Props<T extends string> = {
  id?: string;
  className?: string;
  label?: string;
  size?: ComponentProps<typeof SelectTrigger>["size"];
  options: Option<T>[];
  value: T;
  disabled?: boolean;
  onChange: (value: T) => void;
};

export function OptionSelect<T extends string>({
  id,
  className,
  label,
  size = "sm",
  options,
  value,
  disabled,
  onChange,
}: Props<T>) {
  return (
    <Select
      value={toSelectValue(value)}
      disabled={disabled}
      onValueChange={(next) => onChange(fromSelectValue(next) as T)}
    >
      <SelectTrigger id={id} size={size} aria-label={label} className={className}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={toSelectValue(option.value)}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
