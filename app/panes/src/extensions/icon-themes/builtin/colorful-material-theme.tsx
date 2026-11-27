import { Folder, FolderOpen } from '@phosphor-icons/react'
import { materialIconSvg } from './material-icons'
import type { IconThemeDefinition } from '../types'

export const colorfulMaterialIconTheme: IconThemeDefinition = {
  id: 'colorful-material',
  name: 'Colorful Material Icons',
  description: 'Material Design file icons with original colors',
  getFileIcon: (fileName: string, isDir: boolean, isExpanded = false, _isSymlink = false) => {
    if (isDir) {
      const Icon = isExpanded ? FolderOpen : Folder
      return { component: <Icon weight="fill" color="#f2c14e" /> }
    }
    const svg = materialIconSvg(fileName)
    return svg ? { svg } : {}
  },
}
