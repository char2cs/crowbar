import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  applyReplacementCase,
  replaceAllSearchMatches,
  replaceSearchMatch,
} from "@/features/editor/utils/search-replace";
import { createWorkspaceStore } from "@/features/workspace/stores/workspace-store";
import { setActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";
import { ROOT_PANE_ID } from "@/features/panes/constants/pane";

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

describe("search replace utilities", () => {
  it("preserves common match casing when requested", () => {
    expect(applyReplacementCase("bar", "FOO")).toBe("BAR");
    expect(applyReplacementCase("BAR", "foo")).toBe("bar");
    expect(applyReplacementCase("bar", "Foo")).toBe("Bar");
  });

  it("replaces one match and shifts following match offsets", () => {
    expect(
      replaceSearchMatch(
        "one fish two fish",
        [
          { start: 4, end: 8 },
          { start: 13, end: 17 },
        ],
        0,
        "cat",
      ),
    ).toEqual({
      content: "one cat two fish",
      matches: [{ start: 12, end: 16 }],
      currentMatchIndex: 0,
    });
  });

  it("replaces all matches in reverse offset order", () => {
    expect(
      replaceAllSearchMatches(
        "one fish two fish",
        [
          { start: 4, end: 8 },
          { start: 13, end: 17 },
        ],
        "cat",
      ),
    ).toBe("one cat two cat");
  });

  it("preserves case while replacing all matches", () => {
    expect(
      replaceAllSearchMatches(
        "foo Foo FOO",
        [
          { start: 0, end: 3 },
          { start: 4, end: 7 },
          { start: 8, end: 11 },
        ],
        "bar",
        { preserveCase: true },
      ),
    ).toBe("bar Bar BAR");
  });
});

describe("search replace store actions", () => {
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
  });

  afterEach(async () => {
    setActiveWorkspaceStoreRef(null);
    const { useEditorStateStore } = await import("@/features/editor/stores/state-store");
    const { useEditorUIStore } = await import("@/features/editor/stores/ui-store");

    useEditorUIStore.getState().actions.clearSearch();
    useEditorUIStore.setState({
      searchOptions: {
        caseSensitive: false,
        wholeWord: false,
        useRegex: false,
        preserveCase: false,
      },
    });
    useEditorStateStore.setState({
      onChange: () => {},
    });
    vi.unstubAllGlobals();
  });

  it("does not replace all when the match list is limited", async () => {
    const { useEditorStateStore } = await import("@/features/editor/stores/state-store");
    const { useEditorUIStore } = await import("@/features/editor/stores/ui-store");
    const onChange = vi.fn();
    const limitedMatches = [{ start: 0, end: 4 }];

    useEditorStateStore.setState({ onChange });
    useEditorUIStore.setState({
      searchMatches: limitedMatches,
      searchResultsLimited: true,
      currentMatchIndex: 0,
      replaceQuery: "cat",
    });

    useEditorUIStore.getState().actions.replaceAll();

    expect(onChange).not.toHaveBeenCalled();
    expect(useEditorUIStore.getState().searchMatches).toEqual(limitedMatches);
    expect(useEditorUIStore.getState().searchResultsLimited).toBe(true);
  });

  it("preserves replacement case through store replace all", async () => {
    const { useEditorStateStore } = await import("@/features/editor/stores/state-store");
    const { useEditorUIStore } = await import("@/features/editor/stores/ui-store");
    const onChange = vi.fn();

    useEditorStateStore.setState({ onChange });
    useEditorUIStore.setState({
      searchMatches: [
        { start: 0, end: 3 },
        { start: 4, end: 7 },
        { start: 8, end: 11 },
      ],
      searchResultsLimited: false,
      currentMatchIndex: 0,
      replaceQuery: "bar",
      searchOptions: {
        caseSensitive: false,
        wholeWord: false,
        useRegex: false,
        preserveCase: true,
      },
    });

    // Set up active workspace store with a buffer containing the search content
    const wsStore = createWorkspaceStore("test-ws");
    const bufferId = wsStore.getState().bufferActions.openContent({
      type: "editor",
      path: "/tmp/search.txt",
      name: "search.txt",
      content: "foo Foo FOO",
    });
    // Make it the active pane buffer
    wsStore.setState((state) => ({
      ...state,
      activePaneId: ROOT_PANE_ID,
      paneRoot: {
        type: "group",
        id: ROOT_PANE_ID,
        bufferIds: [bufferId],
        activeBufferId: bufferId,
        locked: false,
        previewBufferId: null,
        pinnedBufferIds: [],
      },
    }));
    setActiveWorkspaceStoreRef(wsStore);

    useEditorUIStore.getState().actions.replaceAll();

    expect(onChange).toHaveBeenCalledWith(
      "bar Bar BAR",
      "foo Foo FOO",
      expect.any(Object),
      undefined,
    );
  });
});
