// xterm's keyboard model is complete, and re-emitting a key sends it twice, so the only override is
// modifier+Enter: xterm lacks the kitty protocol TUIs use to tell it from Enter, so we send the
// CSI-u form and the caller suppresses xterm's CR. null lets xterm handle the key.
export function resolveKeyOverride(event: KeyboardEvent): string | null {
  // Only on keydown, and never when Cmd/Ctrl is held (those are app/terminal
  // shortcuts, not text input).
  if (event.type !== 'keydown' || event.metaKey || event.ctrlKey) return null
  if (event.key !== 'Enter') return null
  if (event.shiftKey) return '\x1b[13;2u' // Shift+Enter  → CSI 13 ; 2 u
  if (event.altKey) return '\x1b[13;3u' //   Alt+Enter    → CSI 13 ; 3 u
  return null
}
