import { useEffect, useMemo, type RefObject } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { fileOpenBenchmark } from "@/features/editor/utils/file-open-benchmark";
import { FILE_TREE_DENSITY_CONFIG } from "@/features/file-explorer/lib/file-tree-density";
import {
  buildVisibleFileTreeRows,
  type VisibleFileTreeRow,
} from "@/features/file-explorer/lib/visible-file-tree-rows";
import { useFileTreeStore } from "@/features/file-explorer/stores/file-explorer-tree-store";
import type { FileEntry } from "@/features/file-system/types/app";
import { useSettingsStore } from "@/features/settings/store";

interface UseFileExplorerVisibleRowsOptions {
  files: FileEntry[];
  activePath?: string;
  containerRef: RefObject<HTMLDivElement | null>;
  expandedPathsOverride?: ReadonlySet<string>;
}

export function useFileExplorerVisibleRows({
  files,
  activePath,
  containerRef,
  expandedPathsOverride,
}: UseFileExplorerVisibleRowsOptions) {
  const expandedPaths = useFileTreeStore((state) => state.expandedPaths);
  const compactFolders = useSettingsStore((state) => state.settings.compactFoldersInFileTree);
  const density = useSettingsStore((state) => state.settings.fileTreeDensity);
  const rowHeight = FILE_TREE_DENSITY_CONFIG[density].rowHeight;

  const visibleRows = useMemo(() => {
    return buildVisibleFileTreeRows(files, expandedPathsOverride ?? expandedPaths, {
      compactFolders,
    });
  }, [compactFolders, expandedPaths, expandedPathsOverride, files]);

  const rowVirtualizer = useVirtualizer({
    count: visibleRows.length,
    estimateSize: () => rowHeight,
    getScrollElement: () => containerRef.current,
    overscan: 8,
    // Suppress ResizeObserver callbacks during sidebar/pane drag to prevent 120fps re-renders.
    // On drag end, pane-resize-end event flushes the last deferred measurement.
    observeElementRect: (instance, cb) => {
      const element = instance.scrollElement
      if (!element) return
      let pending: { width: number; height: number } | null = null
      const ro = new ResizeObserver(([entry]) => {
        const { width, height } = entry.contentRect
        const rect = { width: Math.round(width), height: Math.round(height) }
        if (document.documentElement.hasAttribute('data-pane-resizing')) {
          pending = rect
          return
        }
        cb(rect)
        pending = null
      })
      ro.observe(element)
      const r = element.getBoundingClientRect()
      cb({ width: Math.round(r.width), height: Math.round(r.height) })
      const flush = () => {
        if (pending) { cb(pending); pending = null }
      }
      window.addEventListener('pane-resize-end', flush)
      return () => {
        ro.disconnect()
        window.removeEventListener('pane-resize-end', flush)
      }
    },
  });

  useEffect(() => {
    if (!activePath) return;
    if (fileOpenBenchmark.has(activePath)) {
      fileOpenBenchmark.mark(activePath, "visible-rows-sync");
    }
    const index = visibleRows.findIndex((row) => row.file.path === activePath);
    if (index >= 0) {
      if (fileOpenBenchmark.has(activePath)) {
        fileOpenBenchmark.mark(activePath, "visible-row-found", `index=${index}`);
      }
      rowVirtualizer.scrollToIndex(index, { align: "auto" });
    }
  }, [activePath, rowVirtualizer, visibleRows]);

  return { visibleRows, rowVirtualizer };
}

export type VisibleRow = VisibleFileTreeRow;
