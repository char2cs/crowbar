// Stub: sidebar-drag feature is out of scope for this session.
export interface SidebarDragResource {
  type: string
  path?: string
  name: string
  isDir?: boolean
  repoPath?: string
  branch?: string
  commitHash?: string
  message?: string
  author?: string
  date?: string
  filePath?: string
  staged?: boolean
  status?: string
  [key: string]: unknown
}

export function hasSidebarResourceDragData(_dataTransfer: DataTransfer): boolean {
  return false
}

export function readSidebarResourceDragData(
  _dataTransfer: DataTransfer,
): SidebarDragResource | null {
  return null
}

// Global window extension for file drag data and Electron compatibility
declare global {
  interface Window {
    __fileDragData?: SidebarDragResource | null
    electron?: {
      shell: {
        showItemInFolder: (path: string) => void
        openPath: (path: string) => Promise<void>
        openExternal: (url: string) => Promise<void>
      }
      ipcRenderer?: unknown
    }
  }
}

export function dispatchSidebarResourceDropOnAI(_data: SidebarDragResource): void {}
export function writeSidebarResourceDragData(_dataTransfer: DataTransfer, _data: SidebarDragResource): void {}
