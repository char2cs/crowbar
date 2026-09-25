import { Folder, FolderOpen } from '@phosphor-icons/react'
import type { IconThemeDefinition } from '../types'
import { materialIconSvg } from './material-icons'

export const materialIconTheme: IconThemeDefinition = {
  id: 'material',
  name: 'Material Icons',
  description: 'Material Design file icons',
  getFileIcon: (fileName: string, isDir: boolean, isExpanded = false, _isSymlink = false) => {
    if (isDir) {
      const Icon = isExpanded ? FolderOpen : Folder
      return { component: <Icon /> }
    }
    const svg = materialIconSvg(fileName)
    if (!svg) return {}
    const svgContent = svg
      .replace(/fill="[^"]*"/g, 'fill="currentColor"')
      .replace(/stroke="[^"]*"/g, 'stroke="currentColor"')
    return { svg: svgContent }
  },
}
