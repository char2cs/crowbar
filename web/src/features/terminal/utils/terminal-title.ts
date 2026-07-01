/**
 * Derive a clean tab/window title from a raw xterm OSC 0/1/2 title string.
 *
 * A legitimate window title never contains a raw ESC (0x1b): vim, tmux, ssh,
 * Claude Code, etc. set a plain string as the OSC parameter. Some shell prompt
 * configs (e.g. a zsh precmd/preexec hook) instead encode their full
 * ANSI-COLORED prompt *inside* the OSC title parameter. On a session-restore
 * replay the daemon re-emits the whole scrollback at once, so the LAST such
 * title sequence wins and the tab name becomes a garbled prompt fragment like
 * `crowbar [01;32m0[00m] % [?1h= ls -larph >`.
 *
 * Defense in three layers:
 *  1. If the raw title contains ESC, the shell embedded escapes — reject it
 *     outright (return ''), so the caller keeps the previous title / falls back
 *     to the directory or command label.
 *  2. Otherwise strip any stray C0/DEL/C1 control bytes and trim.
 *  3. xterm's OSC parser sometimes DROPS the ESC byte from an embedded prompt,
 *     leaving only the printable *bodies* of the escape sequences behind — e.g.
 *     `Rabbyte [01;32m0[00m] % [?1h=`. That passes layer 1 (no ESC) and layer 2
 *     (the bodies are printable), so also reject any leftover CSI body: a '['
 *     followed by optional digits/';'/'?' and a CSI final byte (SGR 'm', modes
 *     'h'/'l', erase 'K'/'J', cursor 'H'). Legit bracketed titles like "[WIP]"
 *     or "build [2/5]" don't end a bracket group in one of those bytes.
 *
 * Returns '' when there is no usable title (caller should treat that as
 * "no update").
 */
export function sanitizeTerminalTitle(rawTitle: string): string {
  if (rawTitle.includes('\x1b')) return ''

  let out = ''
  for (const ch of rawTitle) {
    const code = ch.charCodeAt(0)
    // Drop C0 controls (0–31), DEL (127), and the C1 CSI introducer (155).
    if (code <= 31 || code === 127 || code === 155) continue
    out += ch
  }
  out = out.trim()

  if (/\[[\d;?]*[mhlKJH]/.test(out)) return ''

  return out
}
