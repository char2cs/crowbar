import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EditorContent } from "@/features/panes/types/pane-content";
import { createWorkspaceStore } from "@/features/workspace/stores/workspace-store";
import { setActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";
import type { WorkspaceStore } from "@/features/workspace/stores/workspace-store";
import { ROOT_PANE_ID } from "@/features/panes/constants/pane";

const mocks = vi.hoisted(() => ({
  readFileContent: vi.fn(),
}));

vi.mock("@/features/file-system/controllers/file-operations", async (importOriginal) => {
  const original =
    await importOriginal<typeof import("@/features/file-system/controllers/file-operations")>();
  return {
    ...original,
    readFileContent: mocks.readFileContent,
  };
});

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

function makeDirtyEditorBuffer(): EditorContent {
  return {
    id: "revert-buffer",
    type: "editor",
    path: "/workspace/revert.ts",
    name: "revert.ts",
    content: "draft",
    savedContent: "saved",
    isDirty: true,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    isActive: true,
    language: "typescript",
    tokens: [],
  };
}

describe("editor revert file command", () => {
  let revertActiveFile: () => Promise<void>;
  let wsStore: WorkspaceStore;

  beforeEach(async () => {
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

    mocks.readFileContent.mockResolvedValue("disk");

    const buffer = makeDirtyEditorBuffer();
    wsStore = createWorkspaceStore("test-ws");
    wsStore.setState((state) => ({
      ...state,
      buffers: [buffer],
      activePaneId: ROOT_PANE_ID,
      paneRoot: {
        type: "group",
        id: ROOT_PANE_ID,
        bufferIds: [buffer.id],
        activeBufferId: buffer.id,
        locked: false,
        previewBufferId: null,
        pinnedBufferIds: [],
      },
      panes: {
        ...state.panes,
        [ROOT_PANE_ID]: {
          id: ROOT_PANE_ID,
          type: "group" as const,
          bufferIds: [buffer.id],
          activeBufferId: buffer.id,
        },
      },
    }));
    setActiveWorkspaceStoreRef(wsStore);

    ({ revertActiveFile } = await import("@/features/keymaps/commands/file-command-actions"));
  });

  afterEach(() => {
    setActiveWorkspaceStoreRef(null);
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("reloads the active local editor buffer from disk and clears dirty state", async () => {
    await revertActiveFile();

    expect(mocks.readFileContent).toHaveBeenCalledWith("/workspace/revert.ts");
    const buffer = wsStore.getState().buffers[0];
    expect(buffer).toMatchObject({
      content: "disk",
      isDirty: false,
      savedContent: "disk",
    });
  });
});
