import { useEffect } from 'react'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { TOGGLE_CONSOLE } from '@/features/keymaps/registry'
import { onConsoleMenuToggle, setConsoleMenuChord } from '@/lib/crowbar-bridge'

export function useConsoleKeyboard(onToggle: () => void): void {
  const chordMap = useEffectiveChordMap()
  const chord = chordMap[TOGGLE_CONSOLE] ?? null

  // The host menu item owns the chord on macOS, where the system eats Cmd+`
  // before the webview; AppKit then consumes the keystroke, so the DOM path
  // below and this one never both fire for a single press.
  useEffect(() => {
    void setConsoleMenuChord(chord || null).catch(() => {})
  }, [chord])

  useEffect(() => {
    let disposed = false
    let unlisten: (() => void) | undefined
    void onConsoleMenuToggle(onToggle)
      .then((off) => {
        if (disposed) off()
        else unlisten = off
      })
      .catch(() => {})
    return () => {
      disposed = true
      unlisten?.()
    }
  }, [onToggle])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.repeat) return
      if (!chord || !eventMatchesChord(e, chord)) return
      // Capture phase so the chord wins over Monaco, which swallows keys in its bubble handler.
      e.preventDefault()
      e.stopPropagation()
      onToggle()
    }

    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [chord, onToggle])
}
