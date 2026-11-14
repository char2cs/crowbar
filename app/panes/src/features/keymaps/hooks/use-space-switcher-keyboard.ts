import { useEffect } from 'react'

/** Mod+Shift+{1-9} jumps to that space, 9 to the last (the browser-tab convention). Hardcoded,
 *  not in the chord registry: a position convention, not a remappable command. Bare mod+digit
 *  belongs to the sidebar tabs (use-sidebar-tab-keyboard.ts). */
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
