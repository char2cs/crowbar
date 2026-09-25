import type React from 'react'
import { useCallback, type RefObject } from 'react'
import { extractDroppedFilePaths } from '@/features/file-system/utils/file-system-dropped-paths'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { formatDroppedPathsForTerminal } from '../utils/terminal-file-drop'

const handleDragOver = (event: React.DragEvent<HTMLDivElement>) => {
  if (!Array.from(event.dataTransfer.types).includes('Files')) return
  event.preventDefault()
  event.stopPropagation()
  event.dataTransfer.dropEffect = 'copy'
}

/**
 * Files dropped on the terminal — from the page or, in Tauri, from the OS —
 * are typed into it as shell-quoted paths, and the terminal takes focus.
 */
export function useTerminalFileDrop(
  containerRef: RefObject<HTMLDivElement | null>,
  write: (data: string, origin: string) => void,
  focus: () => void,
) {
  const handleTauriDrop = useCallback(
    (paths: string[]) => {
      const text = formatDroppedPathsForTerminal(paths)
      if (!text) return
      write(text, 'file-drop')
      focus()
    },
    [focus, write],
  )
  useTauriFileDrop(containerRef, handleTauriDrop)

  const handleDrop = useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      const text = formatDroppedPathsForTerminal(extractDroppedFilePaths(event.dataTransfer))
      if (!text) return
      event.preventDefault()
      event.stopPropagation()
      write(text, 'file-drop')
      focus()
    },
    [focus, write],
  )

  return { onDragOver: handleDragOver, onDrop: handleDrop }
}
