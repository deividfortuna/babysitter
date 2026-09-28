import { createContext, useImperativeHandle, useLayoutEffect, useState, type ReactNode, type Ref } from "react";
import type { CodeViewDiffItem, CodeViewScrollTarget, DiffLineAnnotation, FileDiffMetadata } from "@pierre/diffs";

type Item = CodeViewDiffItem<unknown>;

type Props = {
  ref?: Ref<{ scrollTo: (target: CodeViewScrollTarget) => void }>;
  items: readonly Item[];
  options?: { diffStyle?: string; overflow?: string; itemMetrics?: unknown; preferredHighlighter?: string };
  className?: string;
  onScroll?: (scrollTop: number, viewer: { getTopForItem: (id: string) => number | undefined }) => void;
  renderCustomHeader?: (item: Item) => ReactNode;
  renderAnnotation?: (annotation: DiffLineAnnotation<unknown>, item: Item) => ReactNode;
};

export const ITEM_HEIGHT = 100;

export const rendered = { renderers: new Set<unknown>(), options: undefined as unknown };

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

export function CodeView({ ref, items, options, className, onScroll, ...render }: Props) {
  const [scrolledTo, setScrolledTo] = useState<string>();
  useLayoutEffect(() => {
    rendered.renderers.add(render.renderCustomHeader);
    rendered.renderers.add(render.renderAnnotation);
    rendered.options = options;
  });
  useImperativeHandle(ref, () => ({
    scrollTo: (target) => setScrolledTo("id" in target ? target.id : undefined),
  }));
  const viewer = {
    getTopForItem: (id: string) => {
      const index = items.findIndex((item) => item.id === id);
      return index < 0 ? undefined : index * ITEM_HEIGHT;
    },
  };
  return (
    <div
      className={className}
      onScroll={(event) => onScroll?.(event.currentTarget.scrollTop, viewer)}
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

export const WorkerPoolContext = createContext<unknown>(undefined);
