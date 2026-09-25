import { useEffect, useEffectEvent } from 'react'
import { useSettingsStore } from '@/features/settings/store'
import type { TerminalSearchState } from './use-terminal-search'

const DEFAULT_FONT_SIZE = 14

function zoomBy(delta: number): void {
  const { settings, updateSetting } = useSettingsStore.getState()
  updateSetting('terminalFontSize', Math.min(Math.max(settings.terminalFontSize + delta, 8), 32))
}

/**
 * The active terminal's window-level chords: Cmd/Ctrl+F opens its find bar,
 * Escape closes it, Cmd/Ctrl +/-/0 zoom its font. Subscribed once per active
 * session; the handler reads the latest search state through an Effect Event.
 */
export function useTerminalShortcuts({
  isActive,
  container,
  search,
}: {
  isActive: boolean
  container: HTMLElement | null
  search: Pick<TerminalSearchState, 'isVisible' | 'open' | 'close'>
}): void {
  const onWindowKeyDown = useEffectEvent((event: KeyboardEvent) => {
    const isTerminalFocused =
      container?.contains(event.target as Node) || container?.contains(document.activeElement)
    const key = event.key.toLowerCase()

    if (
      (event.ctrlKey || event.metaKey) &&
      key === 'f' &&
      (isTerminalFocused || search.isVisible)
    ) {
      event.preventDefault()
      event.stopPropagation()
      search.open()
    }

    if (event.key === 'Escape' && search.isVisible) {
      event.preventDefault()
      search.close()
    }

    if (isTerminalFocused && (event.ctrlKey || event.metaKey)) {
      if (event.key === '+' || event.key === '=') {
        event.preventDefault()
        zoomBy(2)
      } else if (event.key === '-') {
        event.preventDefault()
        zoomBy(-2)
      } else if (event.key === '0') {
        event.preventDefault()
        useSettingsStore.getState().updateSetting('terminalFontSize', DEFAULT_FONT_SIZE)
      }
    }
  })

  useEffect(() => {
    if (!isActive) return
    const handleKeyDown = (event: KeyboardEvent) => onWindowKeyDown(event)
    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [isActive])
}
