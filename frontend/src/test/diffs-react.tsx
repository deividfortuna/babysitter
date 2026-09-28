import { useImperativeHandle, useState, type ReactNode, type Ref } from "react";
import type { CodeViewDiffItem, CodeViewScrollTarget, DiffLineAnnotation, FileDiffMetadata } from "@pierre/diffs";

type Item = CodeViewDiffItem<unknown>;

type Props = {
  ref?: Ref<{ scrollTo: (target: CodeViewScrollTarget) => void }>;
  items: readonly Item[];
  options?: { diffStyle?: string; overflow?: string };
  className?: string;
  renderCustomHeader?: (item: Item) => ReactNode;
  renderAnnotation?: (annotation: DiffLineAnnotation<unknown>, item: Item) => ReactNode;
};

function lines(diff: FileDiffMetadata): string[] {
  const out: string[] = [];
  for (const hunk of diff.hunks) {
    out.push(hunk.hunkSpecs ?? "@@");
    for (const part of hunk.hunkContent) {
      if (part.type === "context") {
        for (let i = 0; i < part.lines; i++) out.push(` ${diff.additionLines[part.additionLineIndex + i]}`);
        continue;
      }
      for (let i = 0; i < part.deletions; i++) out.push(`-${diff.deletionLines[part.deletionLineIndex + i]}`);
      for (let i = 0; i < part.additions; i++) out.push(`+${diff.additionLines[part.additionLineIndex + i]}`);
    }
  }
  return out;
}

export function CodeView({ ref, items, options, className, ...render }: Props) {
  const [scrolledTo, setScrolledTo] = useState<string>();
  useImperativeHandle(ref, () => ({
    scrollTo: (target) => setScrolledTo("id" in target ? target.id : undefined),
  }));
  return (
    <div
      className={className}
      data-scrolled-to={scrolledTo}
      data-diff-style={options?.diffStyle}
      data-overflow={options?.overflow}
    >
      {items.map((item) => (
        <section key={item.id} aria-label={`File ${item.id}`} data-collapsed={item.collapsed ? "true" : "false"}>
          <header>{render.renderCustomHeader?.(item) ?? <span>{item.fileDiff.name}</span>}</header>
          {item.collapsed ? null : (
            <>
              {(item.annotations ?? [])
                .filter((a) => a.lineNumber === 0)
                .map((a, i) => (
                  <div key={`file-${i}`}>{render.renderAnnotation?.(a, item)}</div>
                ))}
              <pre>{lines(item.fileDiff).join("\n")}</pre>
              {(item.annotations ?? [])
                .filter((a) => a.lineNumber > 0)
                .map((a, i) => (
                  <div key={`line-${i}`} data-side={a.side} data-line={a.lineNumber}>
                    {render.renderAnnotation?.(a, item)}
                  </div>
                ))}
            </>
          )}
        </section>
      ))}
    </div>
  );
}

export function WorkerPoolContextProvider({ children }: { children: ReactNode }) {
  return children;
}
