import { useEffect } from 'react'
import { IS_MAC } from '@/utils/platform'
import { saveActiveBuffer, saveAllDirtyBuffers } from '@/features/editor/lib/buffer-save'

/**
 * Registers global save shortcuts:
 * - mod+S → save the active editor buffer
 * - mod+Shift+S → save all dirty editor buffers
 */
export function useSaveKeyboard() {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const modKey = IS_MAC ? e.metaKey : e.ctrlKey

      if (!modKey || e.repeat) return
      if (e.key !== 's' && e.key !== 'S') return
      if (e.altKey) return

      e.preventDefault()
      if (e.shiftKey) {
        void saveAllDirtyBuffers()
      } else {
        void saveActiveBuffer()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [])
}
