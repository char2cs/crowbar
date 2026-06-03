import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ROOT_PANE_ID } from "@/features/panes/constants/pane";
import { createWorkspaceStore } from "@/features/workspace/stores/workspace-store";
import { setActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";
import { getAllLeafIds } from "@/features/panes/utils/pane-layout";
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

describe("pane split actions", () => {
  let wsStore: WorkspaceStore;

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
    wsStore = createWorkspaceStore("test-ws");
    setActiveWorkspaceStoreRef(wsStore);
  });

  afterEach(() => {
    setActiveWorkspaceStoreRef(null);
    vi.unstubAllGlobals();
  });

  it("creates an adjacent pane and activates it", async () => {
    const { createPaneBeside } = await import("@/features/panes/utils/pane-split-actions");

    const paneId = createPaneBeside(ROOT_PANE_ID, "horizontal");

    expect(paneId).not.toBeNull();
    const rootIds = getAllLeafIds(wsStore.getState().rootLayout);
    expect(rootIds).toHaveLength(2);
    expect(wsStore.getState().activePaneId).toBe(paneId);
  });

  it("can seed the adjacent pane with a shared buffer", async () => {
    const { createPaneBeside } = await import("@/features/panes/utils/pane-split-actions");
    const paneActions = wsStore.getState().paneActions;

    paneActions.addBufferToPane(ROOT_PANE_ID, "buffer-a");

    const paneId = createPaneBeside(ROOT_PANE_ID, "horizontal", "after", "buffer-a");

    expect(paneId).not.toBeNull();
    if (!paneId) return;
    expect(paneActions.getPaneById(paneId)?.bufferIds).toEqual(["buffer-a"]);
  });
});
