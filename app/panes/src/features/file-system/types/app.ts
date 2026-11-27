export interface AppFile {
  path: string
  name: string
  isDirectory?: boolean
  isDir?: boolean
  isFile?: boolean
  isSymlink?: boolean
  symlinkTarget?: string
  children?: AppFile[]
  ignored?: boolean
  isRenaming?: boolean
  isEditing?: boolean
  isNewItem?: boolean
  gitStatus?: string
}
export type FileEntry = AppFile
export interface ContextMenuState<T = unknown> {
  x?: number
  y?: number
  isOpen?: boolean
  position?: { x: number; y: number }
  data?: T | null
  path: string
  isDir: boolean
  isFile?: boolean
  name?: string
}
