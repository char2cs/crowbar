import type React from 'react'

export interface FileIconResult {
  svg?: string | null
  component?: React.ReactNode | null
}

export interface IconThemeDefinition {
  id: string
  name: string
  description?: string
  category?: string
  getFileIcon(
    fileName: string,
    isDir: boolean,
    isExpanded?: boolean,
    isSymlink?: boolean,
  ): FileIconResult
}

export interface IconThemeSource {
  extensionId: string
  isBundled?: boolean
}
