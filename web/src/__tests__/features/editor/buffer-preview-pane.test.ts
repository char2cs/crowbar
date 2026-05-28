import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ROOT_PANE_ID } from "@/features/panes/constants/pane";
import { createWorkspaceStore } from "@/features/workspace/stores/workspace-store";
import { setActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";
import type { WorkspaceStore } from "@/features/workspace/stores/workspace-store";

const createMockStorage = () => {
  const storage = new Map<string, string>();

  return {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => {
      storage.set(key, value);
    },
    removeItem: (key: string) => {
      storage.delete(key);
    },
    clear: () => {
      storage.clear();
    },
    key: (index: number) => Array.from(storage.keys())[index] ?? null,
    get length() {
      return storage.size;
    },
  };
};

describe("buffer preview pane integration", () => {
  let store: WorkspaceStore;

  beforeEach(() => {
    vi.stubGlobal("localStorage", createMockStorage());
    vi.stubGlobal("window", {
      __TAURI_INTERNALS__: {
        invoke: vi.fn().mockResolvedValue([]),
        metadata: {
          currentWindow: { label: "main" },
          currentWebview: { label: "main" },
        },
      },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    });
    store = createWorkspaceStore("test-ws");
    setActiveWorkspaceStoreRef(store);
  });

  afterEach(() => {
    setActiveWorkspaceStoreRef(null);
    vi.unstubAllGlobals();
  });

  it("tracks preview buffer metadata per pane when previews are opened", () => {
    const { bufferActions, paneActions } = store.getState();

    const firstPreviewId = bufferActions.openContent({
      type: "editor",
      path: "/workspace/a.ts",
      name: "a.ts",
      content: "a",
      isPreview: true,
    });
    const rightPaneId = paneActions.splitPane(ROOT_PANE_ID, "horizontal");
    expect(rightPaneId).not.toBeNull();
    if (!rightPaneId) return;

    const secondPreviewId = bufferActions.openContent({
      type: "editor",
      path: "/workspace/b.ts",
      name: "b.ts",
      content: "b",
      isPreview: true,
    });

    expect(store.getState().buffers.map((buffer) => buffer.id)).toEqual([
      firstPreviewId,
      secondPreviewId,
    ]);
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.previewBufferId).toBe(firstPreviewId);
    expect(paneActions.getPaneById(rightPaneId)?.previewBufferId).toBe(secondPreviewId);
  });

  it("clears pane preview metadata when a preview becomes definite", () => {
    const { bufferActions, paneActions } = store.getState();

    const previewId = bufferActions.openContent({
      type: "editor",
      path: "/workspace/preview.ts",
      name: "preview.ts",
      content: "preview",
      isPreview: true,
    });

    expect(paneActions.getPaneById(ROOT_PANE_ID)?.previewBufferId).toBe(previewId);

    bufferActions.promotePreview(previewId);

    expect(
      store.getState().buffers.find((buffer) => buffer.id === previewId)?.isPreview,
    ).toBe(false);
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.previewBufferId).toBeNull();
  });

  it("pins preview buffers as definite pane metadata", () => {
    const { bufferActions, paneActions } = store.getState();

    const previewId = bufferActions.openContent({
      type: "editor",
      path: "/workspace/pinned.ts",
      name: "pinned.ts",
      content: "pinned",
      isPreview: true,
    });

    // Pin: promote preview (clear isPreview) + set pinned on buffer + update pane metadata
    bufferActions.promotePreview(previewId);
    bufferActions.setPinned(previewId, true);
    paneActions.setPaneBufferPinned(ROOT_PANE_ID, previewId, true);

    const buffer = store.getState().buffers.find((item) => item.id === previewId);
    const pane = paneActions.getPaneById(ROOT_PANE_ID);
    expect(buffer?.isPreview).toBe(false);
    expect(buffer?.isPinned).toBe(true);
    expect(pane?.previewBufferId).toBeNull();
    expect(pane?.pinnedBufferIds).toEqual([previewId]);
  });

  it("opens a new tab placeholder in the active pane", () => {
    const { bufferActions, paneActions } = store.getState();

    const editorId = bufferActions.openContent({
      type: "editor",
      path: "/workspace/a.ts",
      name: "a.ts",
      content: "",
    });
    const newTabId = bufferActions.openContent({ type: "newTab" });

    const newTabBuffer = store.getState().buffers.find((buffer) => buffer.id === newTabId);
    expect(newTabBuffer?.type).toBe("newTab");
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.bufferIds).toEqual([editorId, newTabId]);
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.activeBufferId).toBe(newTabId);
  });

  it("opens references as a singleton buffer like diagnostics", () => {
    const { bufferActions } = store.getState();

    const firstReferencesId = bufferActions.openContent({ type: "references" });
    const secondReferencesId = bufferActions.openContent({ type: "references" });
    const diagnosticsId = bufferActions.openContent({ type: "diagnostics" });

    expect(secondReferencesId).toBe(firstReferencesId);
    expect(store.getState().buffers).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: firstReferencesId,
          type: "references",
          path: "references://",
          name: "References",
        }),
        expect.objectContaining({
          id: diagnosticsId,
          type: "diagnostics",
          path: "diagnostics://",
          name: "Problems",
        }),
      ]),
    );
  });
});
