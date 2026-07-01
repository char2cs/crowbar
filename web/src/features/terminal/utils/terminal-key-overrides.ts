/**
 * The ONLY key→sequence overrides this terminal injects manually.
 *
 * xterm.js has a complete, correct keyboard model (Ctrl+U → \x15, Shift+Tab →
 * \x1b[Z, Backspace, arrows, and Option word-nav via macOptionIsMeta, …). The app
 * must NOT re-implement those — emitting them again from a key handler sends every
 * such key twice (the double-fire bug). So this file deliberately contains almost
 * nothing.
 *
 * The one genuine exception is modifier+Enter. TUIs like Claude Code use the kitty
 * keyboard protocol (CSI > 1 u) to tell Shift+Enter / Alt+Enter apart from a plain
 * Enter (newline-in-input vs. submit). xterm.js 5.5.0 does NOT implement that
 * protocol, so it emits a bare CR and the app cannot distinguish them. We emit the
 * CSI-u "fixterms" form ourselves — and the caller SUPPRESSES xterm's default CR
 * (returns false from the customKeyEventHandler) so it is sent exactly once.
 *
 * Remove this entirely if/when xterm.js gains kitty-protocol support.
 *
 * Returns the sequence to send (and a signal to suppress xterm's default), or null
 * to let xterm handle the key normally.
 */
export function resolveKeyOverride(event: KeyboardEvent): string | null {
  // Only on keydown, and never when Cmd/Ctrl is held (those are app/terminal
  // shortcuts, not text input).
  if (event.type !== 'keydown' || event.metaKey || event.ctrlKey) return null
  if (event.key !== 'Enter') return null
  if (event.shiftKey) return '\x1b[13;2u' // Shift+Enter  → CSI 13 ; 2 u
  if (event.altKey) return '\x1b[13;3u' //   Alt+Enter    → CSI 13 ; 3 u
  return null
}
