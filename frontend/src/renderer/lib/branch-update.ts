import type { Option } from "@/components/option-select";
import type { BranchUpdate, Watch } from "@/hooks/useWatches";

export const BRANCH_UPDATES: Option<BranchUpdate>[] = [
  { value: "rebase", label: "Rebase" },
  { value: "merge", label: "Merge" },
];

export const DEPENDABOT_OWNS_BRANCH = "Dependabot owns the branch, so only the bot updates it.";

const OWNER_TEXT: Partial<Record<Watch["branchUpdater"], string>> = {
  dependabot: DEPENDABOT_OWNS_BRANCH,
  session: "Your own coding session pushes the branch, so it updates the branch itself.",
};

export function branchUpdateLabel(value: BranchUpdate): string {
  return BRANCH_UPDATES.find((update) => update.value === value)?.label ?? value;
}

export function branchOwnerText(watch: Pick<Watch, "branchUpdater">): string | undefined {
  return OWNER_TEXT[watch.branchUpdater];
}
