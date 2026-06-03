import { create } from "zustand";
import { getGitBlame } from "../api/git-blame-api";
import type { GitBlame, GitBlameLine } from "../types/git-types";
import { loading, success, failed, idle, dataOf, type Loadable } from "@/lib/loadable";

interface GitBlameState {
  blame: Map<string, Loadable<GitBlame>>;
  fileToRepo: Map<string, string>;
  loadBlameForFile: (repoPath: string, filePath: string) => Promise<void>;
  clearBlameForFile: (filePath: string) => void;
  clearAllBlame: () => void;
  getBlameForLine: (filePath: string, lineNumber: number) => GitBlameLine | null;
  getRepoPath: (filePath: string) => string | null;
}

export const useGitBlameStore = create<GitBlameState>((set, get) => ({
  blame: new Map(),
  fileToRepo: new Map(),

  loadBlameForFile: async (repoPath, filePath) => {
    const current = get().blame.get(filePath);
    if (current && (current.status === "success" || current.status === "loading")) return;
    set({ blame: new Map(get().blame).set(filePath, loading(current ?? idle())) });
    try {
      const data = await getGitBlame(repoPath, filePath);
      if (!data) throw new Error("Failed to load blame data");
      set({
        blame: new Map(get().blame).set(filePath, success(data)),
        fileToRepo: new Map(get().fileToRepo).set(filePath, repoPath),
      });
    } catch (err) {
      set({ blame: new Map(get().blame).set(filePath, failed(err as Error, current ?? idle())) });
    }
  },

  clearBlameForFile: (filePath) => {
    const blame = new Map(get().blame);
    blame.delete(filePath);
    const fileToRepo = new Map(get().fileToRepo);
    fileToRepo.delete(filePath);
    set({ blame, fileToRepo });
  },

  clearAllBlame: () => set({ blame: new Map(), fileToRepo: new Map() }),

  getBlameForLine: (filePath, lineNumber) => {
    const data = dataOf(get().blame.get(filePath));
    if (!data) return null;
    for (const line of data.lines) {
      const start = line.line_number;
      const end = start + line.total_lines - 1;
      if (lineNumber >= start && lineNumber <= end) return line;
    }
    return null;
  },

  getRepoPath: (filePath) => get().fileToRepo.get(filePath) ?? null,
}));
