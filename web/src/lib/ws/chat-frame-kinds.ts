/**
 * The chat frames that say NOTHING about the tree's shape.
 *
 * Listed as the exception rather than listing the structural kinds, and that
 * direction is the point: every one of these is either a turn in flight or a
 * question about which PROCESS is on a chat, and both sets are closed and
 * well-known. Everything else a chat aggregate can emit — created, deleted,
 * title_set, placement_set, order_set, and any placement kind a newer daemon
 * mints — has moved, renamed or removed a ROW, and the sidebar has to be told.
 * Defaulting the unknown kind to "the tree moved" costs one repo-scoped reseed;
 * defaulting it the other way is a row that silently never appears, which is
 * exactly how folders came to not sync across windows.
 *
 * `turn_started`/`turn_stopped`/`message_delta` in particular MUST stay out of
 * the structural set: they are the hottest frames on the feed, and reseeding a
 * whole repo's chat list on each one is a request storm per agent turn.
 */
export const NON_STRUCTURAL_CHAT_KINDS: ReadonlySet<string> = new Set([
  'turn_started',
  'turn_stopped',
  'message_delta',
  'terminal_wait',
  'prompt_settled',
  'session_bound',
  // A snapshot republished for a change no aggregate event names (a phase, an
  // attach, a terminal-wait verdict), and the runner lifecycle: each moves a
  // process, never a row.
  'snapshot',
  'started',
  'moved',
  'displaced',
  'exited',
  // The live views of a turn in progress: the plan, the compaction edge and the
  // usage gauge. Each rides its whole payload and moves no row; treating them
  // as structural reseeded a repo's chat list on every plan restatement.
  'plan',
  'compaction_started',
  'compaction_stopped',
  'telemetry',
  // `worktree_state` belongs here for exactly the reason the three hot kinds
  // above do. It carries the git state of the worktree a chat owns — diff
  // counts, PR state, lock status — and it is emitted from the same push site
  // as every workspace frame, so it fires on each working-tree sync and each
  // provider poll. It moves no row: the frame names a chat that already exists
  // and changes only what is drawn ON it, and the whole payload rides the frame
  // (see AgentChatEvent.Worktree), so there is nothing to re-read. Treating it
  // as structural would reseed a whole repo's chat list every time somebody
  // saved a file.
  'worktree_state',
])
