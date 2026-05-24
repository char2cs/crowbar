// Stub: FileExplorerIcon — will be replaced when file-explorer is fully copied
import { FileText } from '@phosphor-icons/react'

export interface FileExplorerIconProps {
  fileName?: string
  filePath?: string
  isDirectory?: boolean
  /** Athas alias for isDirectory */
  isDir?: boolean
  isExpanded?: boolean
  className?: string
  size?: number
}

export function FileExplorerIcon({ className }: FileExplorerIconProps) {
  return <FileText className={className} />
}
