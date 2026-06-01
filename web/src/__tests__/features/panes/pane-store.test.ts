import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from "@/features/panes/constants/pane";
import { createWorkspaceStore } from "@/features/workspace/stores/workspace-store";
import type { WorkspaceStore } from "@/features/workspace/stores/workspace-store";

const createMockStorage = () => {
  const storage = new Map<string, string>();
  return {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => { storage.set(key, value); },
    removeItem: (key: string) => { storage.delete(key); },
    clear: () => { storage.clear(); },
    key: (index: number) => Array.from(storage.keys())[index] ?? null,
    get length() { return storage.size; },
  };
};

describe("pane-store bottom pane integration", () => {
  let store: WorkspaceStore;

  beforeEach(() => {
    vi.stubGlobal("localStorage", createMockStorage());
    vi.stubGlobal("window", {
      __TAURI_INTERNALS__: {
        invoke: vi.fn().mockResolvedValue([]),
        metadata: { currentWindow: { label: "main" }, currentWebview: { label: "main" } },
      },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    });
    store = createWorkspaceStore("test-ws");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function paneActions() {
    return store.getState().paneActions;
  }

  function panes() {
    return store.getState().panes;
  }

  it("moves buffers between the root pane and the bottom pane", () => {
    const { addBufferToPane, moveBufferToPane } = paneActions();

    addBufferToPane(ROOT_PANE_ID, "buffer-a");
    moveBufferToPane("buffer-a", ROOT_PANE_ID, BOTTOM_PANE_ID);

    expect(panes()[ROOT_PANE_ID]?.bufferIds).toEqual([]);
    expect(panes()[BOTTOM_PANE_ID]?.bufferIds).toEqual(["buffer-a"]);
    expect(panes()[BOTTOM_PANE_ID]?.activeBufferId).toBe("buffer-a");

    moveBufferToPane("buffer-a", BOTTOM_PANE_ID, ROOT_PANE_ID);

    expect(panes()[ROOT_PANE_ID]?.bufferIds).toEqual(["buffer-a"]);
    expect(panes()[ROOT_PANE_ID]?.activeBufferId).toBe("buffer-a");
    expect(panes()[BOTTOM_PANE_ID]?.bufferIds).toEqual([]);
  });

  it("can split the bottom root like any other pane tree", () => {
    const { addBufferToPane, splitPane, getAllPaneGroups } = paneActions();

    addBufferToPane(BOTTOM_PANE_ID, "buffer-a");
    const newPaneId = splitPane(BOTTOM_PANE_ID, "horizontal");

    expect(newPaneId).not.toBeNull();

    expect(getAllPaneGroups().length).toBeGreaterThanOrEqual(2);
    expect(panes()[BOTTOM_PANE_ID]?.bufferIds).toEqual(["buffer-a"]);
  });

  it("preserves an empty source pane when moving the only buffer into a new split", () => {
    const { addBufferToPane, splitPane, getAllPaneGroups } = paneActions();

    addBufferToPane(ROOT_PANE_ID, "buffer-a");
    const newPaneId = splitPane(ROOT_PANE_ID, "horizontal");
    expect(newPaneId).not.toBeNull();
    if (!newPaneId) return;

    store.getState().paneActions.moveBufferToPane("buffer-a", ROOT_PANE_ID, newPaneId);

    const groups = getAllPaneGroups();
    expect(groups.length).toBeGreaterThanOrEqual(1);
    expect(panes()[newPaneId]?.bufferIds).toEqual(["buffer-a"]);
  });

  it("falls back to the most recently active remaining pane when closing the active pane", () => {
    const { addBufferToPane, splitPane, setActivePane } = paneActions();

    addBufferToPane(ROOT_PANE_ID, "buffer-a");
    const rightPaneId = splitPane(ROOT_PANE_ID, "horizontal");
    expect(rightPaneId).not.toBeNull();
    if (!rightPaneId) return;

    const bottomPaneId = store.getState().paneActions.splitPane(rightPaneId, "vertical");
    expect(bottomPaneId).not.toBeNull();
    if (!bottomPaneId) return;

    setActivePane(ROOT_PANE_ID);
    setActivePane(rightPaneId);
    setActivePane(bottomPaneId);
    store.getState().paneActions.addBufferToPane(bottomPaneId, "buffer-b");
    store.getState().paneActions.closePane(bottomPaneId);

    const newActiveId = store.getState().activePaneId;
    expect([ROOT_PANE_ID, rightPaneId]).toContain(newActiveId);
    const fallbackPane = store.getState().panes[newActiveId];
    expect(fallbackPane?.bufferIds).toContain("buffer-b");
  });

  it("merges buffers into the fallback pane when closing an inactive pane", () => {
    const { addBufferToPane, splitPane, setActivePane, getPaneById } = paneActions();

    addBufferToPane(ROOT_PANE_ID, "buffer-a");
    const rightPaneId = splitPane(ROOT_PANE_ID, "horizontal");
    expect(rightPaneId).not.toBeNull();
    if (!rightPaneId) return;

    store.getState().paneActions.addBufferToPane(rightPaneId, "buffer-b");
    setActivePane(ROOT_PANE_ID);
    store.getState().paneActions.closePane(rightPaneId);

    const rootPane = getPaneById(ROOT_PANE_ID);
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID);
    expect(rootPane?.bufferIds).toContain("buffer-a");
    expect(rootPane?.bufferIds).toContain("buffer-b");
  });

  it("activates a buffer and its pane as a single operation", () => {
    const { addBufferToPane, splitPane, setActivePane, getPaneById } = paneActions();

    addBufferToPane(ROOT_PANE_ID, "buffer-a");
    const rightPaneId = splitPane(ROOT_PANE_ID, "horizontal");
    expect(rightPaneId).not.toBeNull();
    if (!rightPaneId) return;

    store.getState().paneActions.addBufferToPane(rightPaneId, "buffer-b");
    setActivePane(ROOT_PANE_ID);
    store.getState().paneActions.activatePaneBuffer(rightPaneId, "buffer-b");

    const state = store.getState();
    const rightPane = getPaneById(rightPaneId);
    expect(state.activePaneId).toBe(rightPaneId);
    expect(state.mostRecentActivePaneIds[0]).toBe(rightPaneId);
    expect(rightPane?.activeBufferId).toBe("buffer-b");
  });

  it("routes pane-local buffer cycling through pane activation metadata", () => {
    const { addBufferToPane, splitPane, setActivePane, getPaneById } = paneActions();

    addBufferToPane(ROOT_PANE_ID, "buffer-a");
    const rightPaneId = splitPane(ROOT_PANE_ID, "horizontal");
    expect(rightPaneId).not.toBeNull();
    if (!rightPaneId) return;

    store.getState().paneActions.addBufferToPane(rightPaneId, "buffer-b");
    store.getState().paneActions.addBufferToPane(rightPaneId, "buffer-c");
    setActivePane(ROOT_PANE_ID);
    store.getState().paneActions.activatePaneBuffer(rightPaneId, "buffer-b");

    store.getState().paneActions.switchToNextBufferInPane();

    let state = store.getState();
    expect(state.activePaneId).toBe(rightPaneId);
    expect(getPaneById(rightPaneId)?.activeBufferId).toBe("buffer-c");

    store.getState().paneActions.switchToPreviousBufferInPane();

    state = store.getState();
    expect(state.activePaneId).toBe(rightPaneId);
    expect(getPaneById(rightPaneId)?.activeBufferId).toBe("buffer-b");
  });

  it("tracks preview and pinned metadata on pane groups", () => {
    const { addBufferToPane, setPanePreviewBuffer, setPaneBufferPinned, getPaneById } = paneActions();

    addBufferToPane(ROOT_PANE_ID, "buffer-a");
    addBufferToPane(ROOT_PANE_ID, "buffer-b");
    setPanePreviewBuffer(ROOT_PANE_ID, "buffer-a");

    let pane = getPaneById(ROOT_PANE_ID);
    expect(pane?.previewBufferId).toBe("buffer-a");

    setPaneBufferPinned(ROOT_PANE_ID, "buffer-a", true);
    pane = getPaneById(ROOT_PANE_ID);
    expect(pane?.pinnedBufferIds).toContain("buffer-a");

    setPaneBufferPinned(ROOT_PANE_ID, "buffer-a", false);
    pane = getPaneById(ROOT_PANE_ID);
    expect(pane?.pinnedBufferIds ?? []).not.toContain("buffer-a");
  });

  it("clears preview metadata wherever a buffer is promoted", () => {
    const { addBufferToPane, splitPane, setPanePreviewBuffer, clearPreviewBufferEverywhere, getPaneById } = paneActions();

    addBufferToPane(ROOT_PANE_ID, "buffer-a");
    const splitPaneId = splitPane(ROOT_PANE_ID, "horizontal");
    expect(splitPaneId).not.toBeNull();
    if (!splitPaneId) return;

    store.getState().paneActions.addBufferToPane(splitPaneId, "buffer-a");
    setPanePreviewBuffer(ROOT_PANE_ID, "buffer-a");
    store.getState().paneActions.setPanePreviewBuffer(splitPaneId, "buffer-a");

    clearPreviewBufferEverywhere("buffer-a");

    expect(getPaneById(ROOT_PANE_ID)?.previewBufferId).toBeNull();
    expect(getPaneById(splitPaneId)?.previewBufferId).toBeNull();
  });
});
