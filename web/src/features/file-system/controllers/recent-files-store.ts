// Stub
import { create } from "zustand"
export const useRecentFilesStore = create(() => ({
  recentFiles: [] as string[],
  addRecentFile: (_path: string) => {},
  addOrUpdateRecentFile: (_path: string, _name?: string) => {},
}))
