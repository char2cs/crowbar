import { clipboardSet, clipboardPaste, clipboardGet, clipboardClear } from '@/lib/crowbar-bridge';
import { create } from "zustand";
import { createSelectors } from "@/utils/zustand-selectors";

export interface ClipboardEntry {
  path: string;
  is_dir: boolean;
}

export interface FileClipboardState {
  entries: ClipboardEntry[];
  operation: "copy" | "cut";
}

export interface PastedEntry {
  source_path: string;
  destination_path: string;
  is_dir: boolean;
}

interface FileClipboardStore {
  clipboard: FileClipboardState | null;
  actions: {
    copy: (entries: ClipboardEntry[]) => Promise<void>;
    cut: (entries: ClipboardEntry[]) => Promise<void>;
    paste: (targetDirectory: string) => Promise<PastedEntry[]>;
    clear: () => Promise<void>;
    setClipboard: (state: FileClipboardState | null) => void;
  };
}

const useFileClipboardStoreBase = create<FileClipboardStore>()((set) => ({
  clipboard: null,
  actions: {
    copy: async (entries: ClipboardEntry[]) => {
      await clipboardSet(entries, "copy");
      set({ clipboard: { entries, operation: "copy" } });
    },
    cut: async (entries: ClipboardEntry[]) => {
      await clipboardSet(entries, "cut");
      set({ clipboard: { entries, operation: "cut" } });
    },
    paste: async (targetDirectory: string) => {
      const bridgeResult = await clipboardPaste(targetDirectory);
      // Map crowbar-bridge PastedEntry to local PastedEntry type
      const result: PastedEntry[] = bridgeResult.map((r) => ({
        source_path: r.path,
        destination_path: r.path,
        is_dir: false,
      }));
      const bridgeClipboard = await clipboardGet();
      const clipboard: FileClipboardState | null = bridgeClipboard
        ? { entries: bridgeClipboard.entries, operation: bridgeClipboard.operation }
        : null;
      set({ clipboard });
      return result;
    },
    clear: async () => {
      await clipboardClear();
      set({ clipboard: null });
    },
    setClipboard: (state: FileClipboardState | null) => {
      set({ clipboard: state });
    },
  },
}));

export const useFileClipboardStore = createSelectors(useFileClipboardStoreBase);
