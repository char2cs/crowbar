import { SIDEBAR_FILE_ROW_HEIGHT } from '@/components/sidebar/lib/sidebar-file-row-height'

export type FileTreeDensity = 'compact' | 'default' | 'comfortable'

export const FILE_TREE_DENSITY_OPTIONS: Array<{ value: FileTreeDensity; label: string }> = [
  { value: 'compact', label: 'Compact' },
  { value: 'default', label: 'Default' },
  { value: 'comfortable', label: 'Comfortable' },
]

export const DEFAULT_FILE_TREE_DENSITY: FileTreeDensity = 'default'

export function isFileTreeDensity(value: string): value is FileTreeDensity {
  return FILE_TREE_DENSITY_OPTIONS.some((option) => option.value === value)
}

export function normalizeFileTreeDensity(value: string): FileTreeDensity {
  return isFileTreeDensity(value) ? value : DEFAULT_FILE_TREE_DENSITY
}

// Row pitch per density; the rows and their virtualiser both read it.
export const FILE_TREE_DENSITY_CONFIG: Record<FileTreeDensity, { rowHeight: number }> = {
  compact: { rowHeight: 20 },
  default: { rowHeight: SIDEBAR_FILE_ROW_HEIGHT },
  comfortable: { rowHeight: 30 },
}
