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
    expect(s.borderTopLeftRadius).toBe("var(--radius-xl)");
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
    expect(s.borderTopLeftRadius).toBe("var(--radius-xl)");
    expect(s.borderTopRightRadius).toBe("var(--radius-xl)");
    expect(s.borderBottomLeftRadius).toBe("0");
    expect(s.borderBottomRightRadius).toBe("0");
    expect(s.borderRight).toBe("1px solid var(--border)"); // faces gap
    expect(s.borderBottom).toBe("none");                    // window edge
  });

  it("H-split right pane: TL rounded only; right+bottom hidden", () => {
    const pos: PanePosition = { atLeft: false, atTop: true, atRight: true, atBottom: true };
    const s = buildPaneContentStyle(pos, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-xl)");
    expect(s.borderTopRightRadius).toBe("0");
    expect(s.borderBottomLeftRadius).toBe("0");
    expect(s.borderRight).toBe("none");
  });

  it("V-split top pane: TL+BL rounded; right hidden", () => {
    const pos: PanePosition = { atLeft: true, atTop: true, atRight: true, atBottom: false };
    const s = buildPaneContentStyle(pos, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-xl)");
    expect(s.borderTopRightRadius).toBe("0");
    expect(s.borderBottomLeftRadius).toBe("var(--radius-xl)");
    expect(s.borderBottomRightRadius).toBe("0");
    expect(s.borderBottom).toBe("1px solid var(--border)"); // faces gap below
  });

  it("V-split bottom pane: TL rounded; right+bottom hidden", () => {
    const pos: PanePosition = { atLeft: true, atTop: false, atRight: true, atBottom: true };
    const s = buildPaneContentStyle(pos, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-xl)");
    expect(s.borderTopRightRadius).toBe("0");
    expect(s.borderBottomLeftRadius).toBe("0");
  });

  it("interior pane (not at any edge): all 4 corners rounded, no border hidden", () => {
    const s = buildPaneContentStyle(notAtEdge, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-xl)");
    expect(s.borderTopRightRadius).toBe("var(--radius-xl)");
    expect(s.borderBottomLeftRadius).toBe("var(--radius-xl)");
    expect(s.borderBottomRightRadius).toBe("var(--radius-xl)");
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
    expect(s.borderTopRightRadius).toBe("var(--radius-xl)");
    expect(s.borderBottomLeftRadius).toBe("0");
    expect(s.borderBottomRightRadius).toBe("0");
    expect(s.borderLeft).toBe("none");   // window edge
    expect(s.borderRight).toBe("1px solid var(--border)"); // chrome side
    expect(s.borderBottom).toBe("none"); // window edge
  });

  it("H-split right pane (at sidebar): TL+TR rounded", () => {
    const pos: PanePosition = { atLeft: false, atTop: true, atRight: true, atBottom: true };
    const s = buildPaneContentStyle(pos, sidebar);
    expect(s.borderTopLeftRadius).toBe("var(--radius-xl)");
    expect(s.borderTopRightRadius).toBe("var(--radius-xl)");
  });
});
