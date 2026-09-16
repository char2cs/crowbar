import { Folder, FolderOpen } from '@phosphor-icons/react'
import { getIcon } from 'material-file-icons'
import type { IconThemeDefinition } from '../types'

export const colorfulMaterialIconTheme: IconThemeDefinition = {
  id: 'colorful-material',
  name: 'Colorful Material Icons',
  description: 'Material Design file icons with original colors',
  getFileIcon: (fileName: string, isDir: boolean, isExpanded = false, _isSymlink = false) => {
    if (isDir) {
      const Icon = isExpanded ? FolderOpen : Folder
      // `color` (not `className`) so the tint survives FileExplorerIcon's
      // cloneElement, which overwrites className with the caller's own
      // (text-muted-foreground) class but leaves other props alone.
      return { component: <Icon weight="fill" color="#f2c14e" /> }
    }
    const icon = getIcon(fileName)
    // Keep original colors — do not replace fill/stroke
    return { svg: icon.svg }
  },
}
