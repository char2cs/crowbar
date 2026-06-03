# Pane Content Area Intelligent Borders Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Apply an adaptive card-style border and corner rounding to each pane's content area div, where corners round only when neither adjacent edge is an outer window boundary, and the active sidebar side is treated as chrome (not a window edge).

**Architecture:** A pure-function utility (`pane-border.ts`) computes inline border styles from a `PanePosition` struct (four booleans: atLeft/atTop/atRight/atBottom). `PaneNodeRenderer` propagates these flags top-down during tree traversal. `PaneContainer` reads the flags and applies the computed styles to its existing content area div. `SplitViewRoot` seeds the root position and wires in `isBottomPaneVisible`.

**Tech Stack:** React, TypeScript, Tailwind CSS (inline styles for dynamic values), Vitest

---

## File Map

| Status | Path | Purpose |
|--------|------|---------|
| Modify | `web/src/features/panes/types/pane.ts` | Add `PanePosition` type |
| Create | `web/src/features/panes/utils/pane-border.ts` | `isWindowEdge`, `buildPaneContentStyle` — pure, testable |
| Create | `web/src/__tests__/features/panes/utils/pane-border.test.ts` | Unit tests for the above |
| Modify | `web/src/features/panes/components/pane-node-renderer.tsx` | Add `position` prop; compute `childPosition` per flat entry; pass down |
| Modify | `web/src/features/panes/components/pane-container.tsx` | Add `position` prop; apply `buildPaneContentStyle` to the content area div |
| Modify | `web/src/features/panes/components/split-view-root.tsx` | Read `isBottomPaneVisible`; pass root `PanePosition` to `PaneNodeRenderer` |

---

## Task 1: Add `PanePosition` type

**Files:**
- Modify: `web/src/features/panes/types/pane.ts`

- [ ] **Step 1: Add the type**

Append at the end of `web/src/features/panes/types/pane.ts`:

```typescript
export interface PanePosition {
  /** Left edge of this pane touches the absolute left of the content area. */
  atLeft: boolean;
  /** Top edge touches the absolute top of the content area (below tab bar). */
  atTop: boolean;
  /** Right edge touches the absolute right of the content area. */
  atRight: boolean;
  /** Bottom edge touches the absolute bottom (no visible pane below). */
  atBottom: boolean;
}

export const ROOT_PANE_POSITION: PanePosition = {
  atLeft: true,
  atTop: true,
  atRight: true,
  atBottom: true,
};
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors (or only pre-existing errors unrelated to this change).

- [ ] **Step 3: Commit**

```bash
git add web/src/features/panes/types/pane.ts
git commit -m "feat: add PanePosition type for intelligent pane borders"
```

---

## Task 2: Create `pane-border.ts` utility (TDD)

**Files:**
- Create: `web/src/features/panes/utils/pane-border.ts`
- Create: `web/src/__tests__/features/panes/utils/pane-border.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `web/src/__tests__/features/panes/utils/pane-border.test.ts`:

```typescript
import { describe, expect, it } from "vitest";
import { buildPaneContentStyle, isWindowEdge } from "@/features/panes/utils/pane-border";
import type { PanePosition } from "@/features/panes/types/pane";

const full: PanePosition = { atLeft: true, atTop: true, atRight: true, atBottom: true };
const notAtEdge: PanePosition = { atLeft: false, atTop: false, atRight: false, atBottom: false };

describe("isWindowEdge", () => {
  it("top is never a window edge", () => {
    expect(isWindowEdge("top", full, "left")).toBe(false);
    expect(isWindowEdge("top", full, "right")).toBe(false);
    expect(isWindowEdge("top", notAtEdge, "left")).toBe(false);
  });

  it("bottom is always a window edge when atBottom", () => {
    expect(isWindowEdge("bottom", full, "left")).toBe(true);
    expect(isWindowEdge("bottom", full, "right")).toBe(true);
    expect(isWindowEdge("bottom", notAtEdge, "left")).toBe(false);
  });

  it("left is window edge when atLeft and sidebar is NOT on left", () => {
    expect(isWindowEdge("left", { ...full, atLeft: true }, "right")).toBe(true);
    expect(isWindowEdge("left", { ...full, atLeft: true }, "left")).toBe(false);
    expect(isWindowEdge("left", { ...full, atLeft: false }, "right")).toBe(false);
  });

  it("right is window edge when atRight and sidebar is NOT on right", () => {
    expect(isWindowEdge("right", { ...full, atRight: true }, "left")).toBe(true);
    expect(isWindowEdge("right", { ...full, atRight: true }, "right")).toBe(false);
    expect(isWindowEdge("right", { ...full, atRight: false }, "left")).toBe(false);
  });
});

describe("buildPaneContentStyle — left sidebar", () => {
  const sidebar = "left" as const;

  it("single pane: only TL rounded; right+bottom border hidden", () => {
    const s = buildPaneContentStyle(full, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-sm)");
    expect(s.borderTopRightRadius).toBe("0");
    expect(s.borderBottomLeftRadius).toBe("0");
    expect(s.borderBottomRightRadius).toBe("0");
    expect(s.borderTop).toBe("none");
    expect(s.borderLeft).toBe("1px solid var(--border)");   // chrome side: visible
    expect(s.borderRight).toBe("none");                      // window edge
    expect(s.borderBottom).toBe("none");                     // window edge
  });

  it("H-split left pane: TL+TR rounded; bottom hidden", () => {
    const pos: PanePosition = { atLeft: true, atTop: true, atRight: false, atBottom: true };
    const s = buildPaneContentStyle(pos, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-sm)");
    expect(s.borderTopRightRadius).toBe("var(--radius-sm)");
    expect(s.borderBottomLeftRadius).toBe("0");
    expect(s.borderBottomRightRadius).toBe("0");
    expect(s.borderRight).toBe("1px solid var(--border)"); // faces gap
    expect(s.borderBottom).toBe("none");                    // window edge
  });

  it("H-split right pane: TL rounded only; right+bottom hidden", () => {
    const pos: PanePosition = { atLeft: false, atTop: true, atRight: true, atBottom: true };
    const s = buildPaneContentStyle(pos, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-sm)");
    expect(s.borderTopRightRadius).toBe("0");
    expect(s.borderBottomLeftRadius).toBe("0");
    expect(s.borderRight).toBe("none");
  });

  it("V-split top pane: TL+BL rounded; right hidden", () => {
    const pos: PanePosition = { atLeft: true, atTop: true, atRight: true, atBottom: false };
    const s = buildPaneContentStyle(pos, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-sm)");
    expect(s.borderTopRightRadius).toBe("0");
    expect(s.borderBottomLeftRadius).toBe("var(--radius-sm)");
    expect(s.borderBottomRightRadius).toBe("0");
    expect(s.borderBottom).toBe("1px solid var(--border)"); // faces gap below
  });

  it("V-split bottom pane: TL rounded; right+bottom hidden", () => {
    const pos: PanePosition = { atLeft: true, atTop: false, atRight: true, atBottom: true };
    const s = buildPaneContentStyle(pos, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-sm)");
    expect(s.borderTopRightRadius).toBe("0");
    expect(s.borderBottomLeftRadius).toBe("0");
  });

  it("interior pane (not at any edge): all 4 corners rounded, no border hidden", () => {
    const s = buildPaneContentStyle(notAtEdge, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-sm)");
    expect(s.borderTopRightRadius).toBe("var(--radius-sm)");
    expect(s.borderBottomLeftRadius).toBe("var(--radius-sm)");
    expect(s.borderBottomRightRadius).toBe("var(--radius-sm)");
    expect(s.borderLeft).toBe("1px solid var(--border)");
    expect(s.borderRight).toBe("1px solid var(--border)");
    expect(s.borderBottom).toBe("1px solid var(--border)");
  });
});

describe("buildPaneContentStyle — right sidebar (mirror)", () => {
  const sidebar = "right" as const;

  it("single pane: only TR rounded; left+bottom border hidden", () => {
    const s = buildPaneContentStyle(full, sidebar);
    expect(s.borderTopLeftRadius).toBe("0");
    expect(s.borderTopRightRadius).toBe("var(--radius-sm)");
    expect(s.borderBottomLeftRadius).toBe("0");
    expect(s.borderBottomRightRadius).toBe("0");
    expect(s.borderLeft).toBe("none");   // window edge
    expect(s.borderRight).toBe("1px solid var(--border)"); // chrome side
    expect(s.borderBottom).toBe("none"); // window edge
  });

  it("H-split right pane (at sidebar): TL+TR rounded", () => {
    const pos: PanePosition = { atLeft: false, atTop: true, atRight: true, atBottom: true };
    const s = buildPaneContentStyle(pos, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-sm)");
    expect(s.borderTopRightRadius).toBe("var(--radius-sm)");
  });
});
```

- [ ] **Step 2: Run tests — expect them to fail**

```bash
cd web && npx vitest run src/__tests__/features/panes/utils/pane-border.test.ts 2>&1 | tail -10
```

Expected: `FAIL` — "Cannot find module '@/features/panes/utils/pane-border'"

- [ ] **Step 3: Create `pane-border.ts`**

Create `web/src/features/panes/utils/pane-border.ts`:

```typescript
import type { CSSProperties } from "react";
import type { PanePosition } from "../types/pane";

type Edge = "left" | "top" | "right" | "bottom";

export function isWindowEdge(
  edge: Edge,
  position: PanePosition,
  sidebarSide: "left" | "right",
): boolean {
  switch (edge) {
    case "top":
      return false;
    case "left":
      return position.atLeft && sidebarSide !== "left";
    case "right":
      return position.atRight && sidebarSide !== "right";
    case "bottom":
      return position.atBottom;
  }
}

export function buildPaneContentStyle(
  position: PanePosition,
  sidebarSide: "left" | "right",
): CSSProperties {
  const we = (edge: Edge) => isWindowEdge(edge, position, sidebarSide);
  const BORDER = "1px solid var(--border)";
  const NONE = "none";
  const R = "var(--radius-sm)";
  const ZERO = "0";

  return {
    borderTop: NONE,
    borderLeft: we("left") ? NONE : BORDER,
    borderRight: we("right") ? NONE : BORDER,
    borderBottom: we("bottom") ? NONE : BORDER,
    borderTopLeftRadius: we("left") ? ZERO : R,
    borderTopRightRadius: we("right") ? ZERO : R,
    borderBottomLeftRadius: we("left") || we("bottom") ? ZERO : R,
    borderBottomRightRadius: we("right") || we("bottom") ? ZERO : R,
  };
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
cd web && npx vitest run src/__tests__/features/panes/utils/pane-border.test.ts 2>&1 | tail -10
```

Expected: all tests `PASS`.

- [ ] **Step 5: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no new errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/panes/utils/pane-border.ts \
        web/src/__tests__/features/panes/utils/pane-border.test.ts
git commit -m "feat: add pane border utility with isWindowEdge and buildPaneContentStyle"
```

---

## Task 3: Update `PaneNodeRenderer` to propagate position

**Files:**
- Modify: `web/src/features/panes/components/pane-node-renderer.tsx`

The renderer already uses `flattenPaneSplit` to produce a flat list of entries for a single split level. Each entry's position is computed from the parent position, the entry's index, and the split direction.

- [ ] **Step 1: Add `position` prop and `childPosition` helper**

Replace the existing content of `web/src/features/panes/components/pane-node-renderer.tsx` with:

```typescript
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
              {entry.node.type === "split" && entry.node.direction !== node.direction ? (
                <PaneNodeRenderer
                  node={entry.node}
                  hiddenPaneId={hiddenPaneId}
                  position={entryPosition}
                />
              ) : entry.node.type === "group" ? (
                entry.node.id === hiddenPaneId ? (
                  <div className="h-full w-full bg-background" aria-hidden="true" />
                ) : (
                  <PaneContainer pane={entry.node} position={entryPosition} />
                )
              ) : (
                <PaneNodeRenderer
                  node={entry.node}
                  hiddenPaneId={hiddenPaneId}
                  position={entryPosition}
                />
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
```

- [ ] **Step 2: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: errors only about `position` prop on `PaneContainer` not existing yet (that's fine — Task 4 fixes it). If you see other errors, fix them before continuing.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/panes/components/pane-node-renderer.tsx
git commit -m "feat: propagate PanePosition through pane tree in PaneNodeRenderer"
```

---

## Task 4: Update `PaneContainer` to apply border styles

**Files:**
- Modify: `web/src/features/panes/components/pane-container.tsx`

The content area div is at line 1017: `<div className="relative min-h-0 flex-1 overflow-hidden bg-background">`. This is the only div that changes.

- [ ] **Step 1: Add `position` to `PaneContainerProps` and import utilities**

In `web/src/features/panes/components/pane-container.tsx`:

Add to the imports block (near line 21 where `useSettingsStore` is already imported):

```typescript
import { buildPaneContentStyle } from "../utils/pane-border";
import { ROOT_PANE_POSITION, type PanePosition } from "../types/pane";
```

Replace the `PaneContainerProps` interface (currently at line 105):

```typescript
interface PaneContainerProps {
  pane: PaneGroup;
  position?: PanePosition;
}
```

Replace the function signature (currently at line 243):

```typescript
export function PaneContainer({ pane, position = ROOT_PANE_POSITION }: PaneContainerProps) {
```

- [ ] **Step 2: Read sidebarPosition from settings store**

Inside `PaneContainer`, after the existing `useSettingsStore` selector (line ~266), add:

```typescript
const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition);
```

- [ ] **Step 3: Apply styles to the content area div**

Find the content area div (at approximately line 1017):

```typescript
<div className="relative min-h-0 flex-1 overflow-hidden bg-background">
```

Replace it with:

```typescript
<div
  className="relative min-h-0 flex-1 overflow-hidden bg-background"
  style={buildPaneContentStyle(position, sidebarPosition)}
>
```

- [ ] **Step 4: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/panes/components/pane-container.tsx
git commit -m "feat: apply intelligent border/corner styles to pane content area"
```

---

## Task 5: Wire `SplitViewRoot` with bottom pane visibility

**Files:**
- Modify: `web/src/features/panes/components/split-view-root.tsx`

The root `PaneNodeRenderer` call currently uses the default `ROOT_PANE_POSITION` (all `true`). We need to pass `atBottom: !isBottomPaneVisible` so that root panes touching the bottom don't get a bottom border or rounded bottom corners when the terminal panel is open.

- [ ] **Step 1: Import `useUIState` and `ROOT_PANE_POSITION`**

In `web/src/features/panes/components/split-view-root.tsx`, add to imports:

```typescript
import { useUIState } from "@/features/window/stores/ui-state-store";
import { ROOT_PANE_POSITION } from "../types/pane";
```

- [ ] **Step 2: Read `isBottomPaneVisible` and build root position**

Inside `SplitViewRoot`, after the existing `usePaneActions` call, add:

```typescript
const isBottomPaneVisible = useUIState((state) => state.isBottomPaneVisible);
const rootPosition = { ...ROOT_PANE_POSITION, atBottom: !isBottomPaneVisible };
```

- [ ] **Step 3: Pass `rootPosition` to the root `PaneNodeRenderer`**

Find the existing `<PaneNodeRenderer node={root} hiddenPaneId={fullscreenPaneId} />` call and replace with:

```typescript
<PaneNodeRenderer node={root} hiddenPaneId={fullscreenPaneId} position={rootPosition} />
```

- [ ] **Step 4: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 5: Run full test suite**

```bash
cd web && npx vitest run 2>&1 | tail -15
```

Expected: all tests pass (the new pane-border tests plus any pre-existing tests).

- [ ] **Step 6: Commit**

```bash
git add web/src/features/panes/components/split-view-root.tsx
git commit -m "feat: seed root PanePosition with bottom pane visibility in SplitViewRoot"
```

---

## Task 6: Visual verification

Start the dev server and verify the borders look correct in the browser.

- [ ] **Step 1: Start dev server**

```bash
cd web && npm run dev
```

Open the app in a browser.

- [ ] **Step 2: Check single-pane layout (sidebar left)**

Expected: content area has a subtle left border and top-left rounded corner only. No border on right or bottom.

- [ ] **Step 3: Split horizontally (cmd+\ or drag)**

Expected: left pane has TL+TR rounded, right pane has TL only. Visible border on the gap between them.

- [ ] **Step 4: Split vertically**

Expected: top pane has TL+BL rounded, bottom pane has TL only.

- [ ] **Step 5: Open terminal (bottom pane)**

Expected: root panes' bottom borders disappear and bottom corners go square, since `atBottom` is now `false` (bottom pane is visible).

- [ ] **Step 6: Switch sidebar to right (Settings → Appearance → Sidebar Position)**

Expected: all corner rounding mirrors — single pane shows TR only, H-split right pane has TL+TR, etc.

- [ ] **Step 7: Final commit**

```bash
git add -p  # stage only if there are any fixup changes
git commit -m "fix: pane border visual tweaks from manual testing" --allow-empty
```

(Use `--allow-empty` only if there were no fixups; otherwise stage and commit normally.)
