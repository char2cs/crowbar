/**
 * The chat frames that say nothing about the tree's shape: a turn in flight or
 * which process is on a chat. Any other kind moved a row, and the sidebar
 * reseeds; an unknown kind defaulting to structural costs one reseed, never a
 * row that silently never appears.
 */
export const NON_STRUCTURAL_CHAT_KINDS: ReadonlySet<string> = new Set([
  'turn_started',
  'turn_stopped',
  'terminal_wait',
  'session_bound',
  'snapshot',
  'started',
  'displaced',
  'exited',
  'telemetry',
  // Emitted on each working-tree sync; it changes what is drawn on a chat, and
  // its whole payload rides the frame.
  'worktree_state',
])
