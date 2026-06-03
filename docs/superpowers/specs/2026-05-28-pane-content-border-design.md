# Pane Content Area — Intelligent Border & Corner Design

**Date:** 2026-05-28  
**Branch:** `enhancement/design-language`

---

## Goal

Apply a coss/ui-style card border and rounded corners to each pane's **content area** (the editor/terminal/viewer div below the tab bar). Corners round only where neither adjacent edge is an outer window boundary. Borders are hidden on outer window edges. The effect adapts automatically to split layout and sidebar position.

---

## Scope

- **In scope:** border + border-radius on the content area div inside `PaneContainer`; position computation in `PaneNodeRenderer`; sidebar-position awareness.
- **Out of scope:** the tab bar itself (it stays unchanged); bottom pane area (same rule applies but treated as a separate concern; see note below); fullscreen overlay pane (already has its own border treatment).

---

## Key Definitions

**Chrome edge** — an edge that abuts a persistent UI element (tab bar = always top; sidebar = the side it's on).  
**Window edge** — an edge at the outermost content boundary with no chrome on that side.

With sidebar on **left**: chrome = left + top; window = right + bottom.  
With sidebar on **right**: chrome = right + top; window = left + bottom.

---

## Position Flags

For each leaf pane in the split tree, four boolean flags are computed during tree traversal:

| Flag | Meaning |
|------|---------|
| `atLeft` | Pane's left edge is at the absolute left of the content area |
| `atTop` | Pane's top edge is at the absolute top of the content area |
| `atRight` | Pane's right edge is at the absolute right of the content area |
| `atBottom` | Pane's bottom edge is at the absolute bottom of the content area (no visible pane below) |

**Traversal rules** (root starts with all four `true`):

| Split direction | First child inherits | Second child inherits |
|----------------|---------------------|-----------------------|
| horizontal | `atLeft`, `atTop`, `atBottom`; `atRight = false` | `atTop`, `atRight`, `atBottom`; `atLeft = false` |
| vertical   | `atLeft`, `atTop`, `atRight`; `atBottom = false` | `atLeft`, `atRight`, `atBottom`; `atTop = false` |

The flattened-split renderer in `PaneNodeRenderer` must propagate these flags to all `PaneContainer` children.

**Bottom pane note:** Root panes pass `atBottom = !bottomPaneVisible` (the bottom pane area is visible when `bottomRoot` has at least one buffer). Bottom panes themselves always receive `atBottom = true`.

---

## `isWindowEdge` Function

```ts
function isWindowEdge(
  edge: "left" | "top" | "right" | "bottom",
  position: PanePosition,
  sidebarSide: "left" | "right",
): boolean {
  switch (edge) {
    case "top":    return false                                       // always chrome
    case "left":   return position.atLeft  && sidebarSide !== "left"
    case "right":  return position.atRight && sidebarSide !== "right"
    case "bottom": return position.atBottom
  }
}
```

---

## Corner Rounding Rule

A corner rounds when **neither** of its two adjacent edges is a window edge:

```
TL rounds if: !isWindowEdge("top") && !isWindowEdge("left")  →  !isWindowEdge("left")
TR rounds if: !isWindowEdge("top") && !isWindowEdge("right") →  !isWindowEdge("right")
BL rounds if: !isWindowEdge("bottom") && !isWindowEdge("left")
BR rounds if: !isWindowEdge("bottom") && !isWindowEdge("right")
```

Radius value: `var(--radius-sm)` (≈ 6 px, from theme).

---

## Border Visibility Rule

| Edge | Visible when |
|------|-------------|
| top | Never — the pane's own tab bar already provides top separation |
| left | Not a window edge (`!isWindowEdge("left")`) |
| right | Not a window edge (`!isWindowEdge("right")`) |
| bottom | Not a window edge (`!isWindowEdge("bottom")`) |

Border color/width: `1px solid var(--border)` (theme variable, already used elsewhere).

---

## Applied CSS (inline style on the content area div)

```tsx
// In PaneContainer, applied to the `relative min-h-0 flex-1 overflow-hidden` div
const contentStyle = buildPaneContentStyle(position, sidebarSide);
```

```ts
function buildPaneContentStyle(
  position: PanePosition,
  sidebarSide: "left" | "right",
): React.CSSProperties {
  const we = (edge: Edge) => isWindowEdge(edge, position, sidebarSide);
  const BORDER = "1px solid var(--border)";
  const NONE   = "none";
  const R      = "var(--radius-sm)";
  const ZERO   = "0";

  return {
    borderTop:               NONE,
    borderLeft:              we("left")   ? NONE : BORDER,
    borderRight:             we("right")  ? NONE : BORDER,
    borderBottom:            we("bottom") ? NONE : BORDER,
    borderTopLeftRadius:     we("left")   ? ZERO : R,
    borderTopRightRadius:    we("right")  ? ZERO : R,
    borderBottomLeftRadius:  (we("left")  || we("bottom")) ? ZERO : R,
    borderBottomRightRadius: (we("right") || we("bottom")) ? ZERO : R,
  };
}
```

---

## Files to Change

| File | Change |
|------|--------|
| `web/src/features/panes/components/pane-node-renderer.tsx` | Compute `PanePosition` flags during tree traversal; pass to `PaneContainer` |
| `web/src/features/panes/components/pane-container.tsx` | Accept `position` prop; apply `buildPaneContentStyle` to the content area div |
| `web/src/features/panes/types/pane.ts` *(or new file)* | Export `PanePosition` type |
| `web/src/features/panes/utils/pane-border.ts` *(new)* | `buildPaneContentStyle`, `isWindowEdge` — pure functions, easy to unit-test |
| `web/src/features/panes/components/split-view-root.tsx` | Pass `bottomPaneVisible` flag down to root `PaneNodeRenderer` |

---

## Behaviour Table (sidebar left)

| Layout | Pane | Rounded corners |
|--------|------|-----------------|
| Single | only pane | TL |
| H-split | left | TL, TR |
| H-split | right | TL |
| V-split | top | TL, BL |
| V-split | bottom | TL |
| Complex (tall-L + 2 stacked-R) | tall left | TL, TR |
| Complex | top-right | TL, BL |
| Complex | bottom-right | TL |

Right sidebar mirrors symmetrically (TL ↔ TR, BL ↔ BR).

---

## What Does Not Change

- Tab bar height, appearance, or layout
- Resize handle width (4 px) — becomes the visual gap between pane cards
- Fullscreen pane overlay (already handled separately in `SplitViewRoot`)
- The `--border` and `--radius-sm` CSS variable values
