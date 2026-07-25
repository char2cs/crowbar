// Stub
export interface AppFile {
  path: string
  name: string
  isDirectory?: boolean
  /** Alias for isDirectory. */
  isDir?: boolean
  isFile?: boolean
  isSymlink?: boolean
  symlinkTarget?: string
  children?: AppFile[]
  ignored?: boolean
  /** UI state - file is being renamed */
  isRenaming?: boolean
  /** UI state - file is being edited/created */
  isEditing?: boolean
  /** UI state - newly created item not yet saved */
  isNewItem?: boolean
  gitStatus?: string
}
/** Alias used by the file explorer. */
export type FileEntry = AppFile
export interface ContextMenuState<T = unknown> {
  /** Horizontal position */
  x?: number
  /** Vertical position */
  y?: number
  isOpen?: boolean
  position?: { x: number; y: number }
  data?: T | null
  /** File path (file-explorer context menu). */
  path: string
  /** Whether the target is a directory */
  isDir: boolean
  /** Whether the target is a file */
  isFile?: boolean
  name?: string
}
