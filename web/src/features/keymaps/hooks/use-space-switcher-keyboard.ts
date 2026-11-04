import { useEffect } from 'react'

/**
 * Cmd/Ctrl+Shift+{1-9} project switching — the browser-tab convention: 1-8
 * jump to that position, 9 always jumps to the LAST space regardless of how
 * many there are. Fixed, not run through the configurable chord registry
 * (use-workspace-switcher-keyboard.ts's own OPEN_WORKSPACE_SWITCHER) — same
 * reasoning browsers apply to their own Cmd+1-9: a hardcoded position
 * convention, not a remappable command. Bare Cmd/Ctrl+digit belongs to the
 * sidebar modes (use-sidebar-tab-keyboard.ts).
 */
export function useSpaceSwitcherKeyboard(spaceCount: number, onSelect: (index: number) => void) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.repeat) return
      if (!(e.metaKey || e.ctrlKey) || !e.shiftKey || e.altKey) return
      // `code`, not `key`: Shift turns the digit into `!`, `@`, … on most layouts.
      const digit = /^Digit([1-9])$/.exec(e.code)
      if (!digit) return
      if (spaceCount === 0) return
      const requested = Number(digit[1])
      // 9 always means "the last one," even with fewer than 9 spaces open —
      // 1-8 mean that literal position and simply do nothing past the end
      // (real browser tabs don't clamp those to the last tab either).
      const index = requested === 9 ? spaceCount - 1 : requested - 1
      if (index >= spaceCount) return
      e.preventDefault()
      e.stopPropagation()
      onSelect(index)
    }

    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [spaceCount, onSelect])
}
