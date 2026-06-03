/**
 * Tree Cache Store
 * Stores Tree-sitter parse trees per buffer for incremental parsing
 */

import type { Tree } from "web-tree-sitter";
import { create } from "zustand";
import { createSelectors } from "@/utils/zustand-selectors";
import { onActiveWorkspaceStoreChange } from "@/features/workspace/stores/workspace-store-ref";

export interface TreeCacheEntry {
  tree: Tree;
  contentLength: number; // Simple hash for staleness detection
  languageId: string;
  lastUpdated: number;
}

interface TreeCacheState {
  trees: Map<string, TreeCacheEntry>;

  actions: {
    setTree: (bufferId: string, tree: Tree, contentLength: number, languageId: string) => void;
    getTree: (bufferId: string) => TreeCacheEntry | undefined;
    clearTree: (bufferId: string) => void;
    clearAllTrees: () => void;
  };
}

export const useTreeCacheStore = createSelectors(
  create<TreeCacheState>()((set, get) => ({
    trees: new Map(),

    actions: {
      setTree: (bufferId, tree, contentLength, languageId) => {
        set((state) => {
          const newMap = new Map(state.trees);
          // Delete old tree to free memory
          const oldEntry = newMap.get(bufferId);
          if (oldEntry?.tree) {
            try {
              oldEntry.tree.delete();
            } catch {
              // Tree may already be deleted
            }
          }
          newMap.set(bufferId, {
            tree,
            contentLength,
            languageId,
            lastUpdated: Date.now(),
          });
          return { trees: newMap };
        });
      },

      getTree: (bufferId) => {
        return get().trees.get(bufferId);
      },

      clearTree: (bufferId) => {
        set((state) => {
          const newMap = new Map(state.trees);
          const entry = newMap.get(bufferId);
          if (entry?.tree) {
            try {
              entry.tree.delete();
            } catch {
              // Tree may already be deleted
            }
          }
          newMap.delete(bufferId);
          return { trees: newMap };
        });
      },

      clearAllTrees: () => {
        const { trees } = get();
        for (const entry of trees.values()) {
          if (entry?.tree) {
            try {
              entry.tree.delete();
            } catch {
              // Tree may already be deleted
            }
          }
        }
        set({ trees: new Map() });
      },
    },
  })),
);

// Subscribe to workspace store to clean up trees when buffers are closed.
// Uses onActiveWorkspaceStoreChange so the subscription follows the active
// workspace through transitions (mount/unmount of WorkspaceView).
let _stopWatchingRef: (() => void) | null = null;

export function initTreeCacheSubscription() {
  if (_stopWatchingRef) return;

  let _wsSubscriptionUnsub: (() => void) | null = null;

  _stopWatchingRef = onActiveWorkspaceStoreChange((store) => {
    if (_wsSubscriptionUnsub) {
      _wsSubscriptionUnsub();
      _wsSubscriptionUnsub = null;
    }

    if (store) {
      _wsSubscriptionUnsub = store.subscribe((state, prevState) => {
        if (state.buffers === prevState.buffers) return;

        const currentBufferIds = new Set(state.buffers.map((b) => b.id));
        const previousBufferIds = new Set(prevState.buffers.map((b) => b.id));

        // Find closed buffers and clean up their parse trees.
        for (const bufferId of previousBufferIds) {
          if (!currentBufferIds.has(bufferId)) {
            useTreeCacheStore.getState().actions.clearTree(bufferId);
          }
        }
      });
    }
  });
}

/** Reset the subscription guard — only for use in tests. */
export function _resetTreeCacheSubscriptionForTesting(): void {
  if (_stopWatchingRef) {
    _stopWatchingRef();
    _stopWatchingRef = null;
  }
}
