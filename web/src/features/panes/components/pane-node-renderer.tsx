import { useCallback, useMemo } from "react";
import { usePaneActions } from "@/features/workspace/stores/hooks/use-pane-store";
import type { PanePosition } from "../types/pane";
import { ROOT_PANE_POSITION } from "../types/pane";
import type { PaneNode } from "../types/pane";
import { flattenPaneSplit, type FlatPaneEntry } from "../utils/pane-tree";
import { PaneContainer } from "./pane-container";
import { PaneResizeHandle } from "./pane-resize-handle";

interface PaneNodeRendererProps {
  hiddenPaneId?: string | null;
  node: PaneNode;
  position?: PanePosition;
}

interface FlatResizeHandleProps {
  direction: "horizontal" | "vertical";
  index: number;
  entries: FlatPaneEntry[];
  onReset: (index: number) => void;
  onResize: (index: number, sizes: [number, number]) => void;
}

function FlatResizeHandle({ direction, index, entries, onReset, onResize }: FlatResizeHandleProps) {
  const handleResize = useCallback(
    (sizes: [number, number]) => {
      onResize(index, sizes);
    },
    [index, onResize],
  );

  const handleReset = useCallback(() => {
    onReset(index);
  }, [index, onReset]);

  const initialSizes: [number, number] = [entries[index].size, entries[index + 1].size];

  return (
    <PaneResizeHandle
      direction={direction}
      onResize={handleResize}
      onReset={handleReset}
      initialSizes={initialSizes}
    />
  );
}

function childPosition(
  parent: PanePosition,
  index: number,
  total: number,
  direction: "horizontal" | "vertical",
): PanePosition {
  const isFirst = index === 0;
  const isLast = index === total - 1;
  if (direction === "horizontal") {
    return {
      atLeft: isFirst ? parent.atLeft : false,
      atTop: parent.atTop,
      atRight: isLast ? parent.atRight : false,
      atBottom: parent.atBottom,
    };
  }
  return {
    atLeft: parent.atLeft,
    atTop: isFirst ? parent.atTop : false,
    atRight: parent.atRight,
    atBottom: isLast ? parent.atBottom : false,
  };
}

export function PaneNodeRenderer({
  node,
  hiddenPaneId = null,
  position = ROOT_PANE_POSITION,
}: PaneNodeRendererProps) {
  const { distributePaneSplit, resizePaneSplit } = usePaneActions();
  const isHorizontal = node.type === "split" ? node.direction === "horizontal" : false;

  const flatEntries = useMemo(() => {
    if (node.type !== "split") return null;
    return flattenPaneSplit(node);
  }, [node]);

  const handleFlatResize = useCallback(
    (index: number, sizes: [number, number]) => {
      if (node.type !== "split") return;
      resizePaneSplit(node.id, index, sizes);
    },
    [node, resizePaneSplit],
  );

  const handleFlatReset = useCallback(() => {
    if (node.type !== "split") return;
    distributePaneSplit(node.id);
  }, [distributePaneSplit, node]);

  if (node.type === "group") {
    if (hiddenPaneId && node.id === hiddenPaneId) {
      return <div className="h-full w-full bg-background" aria-hidden="true" />;
    }
    return <PaneContainer pane={node} position={position} />;
  }

  if (!flatEntries || flatEntries.length === 0) return null;

  const totalSize = flatEntries.reduce((sum, entry) => sum + entry.size, 0);
  const handleWidth = 4;
  const handleCount = flatEntries.length - 1;
  const direction = node.direction;

  return (
    <div className={`flex h-full w-full ${isHorizontal ? "flex-row" : "flex-col"}`}>
      {flatEntries.map((entry, index) => {
        const pct = (entry.size / totalSize) * 100;
        const handleDeduction = `${(handleWidth * handleCount) / flatEntries.length}px`;
        const entryPosition = childPosition(position, index, flatEntries.length, direction);

        return (
          <div key={entry.node.id} className="contents">
            <div
              className="min-h-0 min-w-0 overflow-hidden"
              style={{
                [isHorizontal ? "width" : "height"]: `calc(${pct}% - ${handleDeduction})`,
              }}
            >
              {entry.node.type === "split" ? (
                <PaneNodeRenderer
                  node={entry.node}
                  hiddenPaneId={hiddenPaneId}
                  position={entryPosition}
                />
              ) : entry.node.id === hiddenPaneId ? (
                <div className="h-full w-full bg-background" aria-hidden="true" />
              ) : (
                <PaneContainer pane={entry.node} position={entryPosition} />
              )}
            </div>
            {index < flatEntries.length - 1 && (
              <FlatResizeHandle
                direction={node.direction}
                index={index}
                entries={flatEntries}
                onReset={handleFlatReset}
                onResize={handleFlatResize}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
