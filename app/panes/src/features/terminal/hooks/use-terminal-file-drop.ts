import { useCallback, type RefObject } from 'react'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { formatDroppedPathsForTerminal } from '../utils/terminal-file-drop'

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
}
