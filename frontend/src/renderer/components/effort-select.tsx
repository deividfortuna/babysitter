import type { ComponentProps } from "react";
import type { ProviderEffort } from "@/hooks/useProviders";
import { OptionSelect, toOptions } from "@/components/option-select";

type Props = Omit<ComponentProps<typeof OptionSelect<string>>, "label" | "options"> & {
  efforts: ProviderEffort[];
  defaultLabel: string;
};

export function EffortSelect({ efforts, defaultLabel, disabled, ...select }: Props) {
  const options = [{ value: "", label: defaultLabel }, ...toOptions(efforts)];
  return <OptionSelect {...select} label="Effort" options={options} disabled={disabled || efforts.length === 0} />;
}
