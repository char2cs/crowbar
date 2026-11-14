import { useEffect } from 'react'
import { APPLE_KEYBOARD } from '@/utils/platform'
import { saveActiveBuffer, saveAllDirtyBuffers } from '@/features/editor/lib/buffer-save'

/** Global mod+S saves the active editor buffer; mod+Shift+S saves every dirty buffer. */
export function useSaveKeyboard() {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const modKey = APPLE_KEYBOARD ? e.metaKey : e.ctrlKey

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
