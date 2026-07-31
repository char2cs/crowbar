import { useCallback, useEffect, useRef } from 'react'
import { Folder } from 'lucide-react'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useRemovalTrayStore, type RemovalEntry } from '@/lib/store/sidebar-removal'
import { ROW_BASE, ROW_GLYPH_BOX, ROW_INACTIVE, ROW_SUB_ACTION } from './workspace-row-base'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { RepoAvatar } from './repo-avatar'
import { commitRemoval, type RemovalNavigate } from './removal-commit'

/**
 * The removal tray at the sidebar's foot: rows on their way out, held where you
 * can still change your mind.
 *
 * The rows are ORDINARY rows — the same 36px box, the same glyph, the same mono
 * label as the row that was dragged. That is the point: a held row has not
 * become a different kind of thing, it is the same row waiting somewhere else.
 * The only additions are a hairline draining along the bottom edge and the
 * seconds it has left, in figures.
 *
 * This replaces the drop-to-delete zone that used to live here. That zone
 * deleted on release with nothing to undo; a tray is what makes the gesture
 * safe enough to be the only removal path in the sidebar.
 */

/** Whole seconds left on a deadline, never below zero. */
function secondsLeft(deadlineAt: number, now: number): number {
  return Math.max(0, Math.ceil((deadlineAt - now) / 1000))
}

/** The count beside a held row: how much else goes with it. */
function GoesWith({ n }: { n: number }) {
  return (
    <span className="shrink-0 rounded-full bg-sidebar-element-idle px-1.5 py-px text-[11px] tabular-nums text-muted-foreground">
      +{n}
    </span>
  )
}

function TrayGlyph({ entry }: { entry: RemovalEntry }) {
  const repo = useSidebarStore((s) => s.repos.find((r) => r.id === entry.repoId))

  if (entry.kind === 'repo') {
    return (
      <RepoAvatar
        avatar={{
          url: repo?.avatarURL,
          label: repo?.avatarLabel ?? entry.label.slice(0, 1).toUpperCase(),
          color: repo?.avatarColor ?? 'bg-neutral-500',
        }}
        name={entry.label}
      />
    )
  }

  if (entry.kind === 'folder') {
    return (
      <span className={ROW_GLYPH_BOX}>
        <Folder aria-hidden="true" className="size-4" />
      </span>
    )
  }

  const workspace = repo?.workspaces.find((w) => w.id === entry.id)
  return (
    <span className={ROW_GLYPH_BOX}>
      <WorkspaceBranchIcon status={workspace?.status ?? 'new'} />
    </span>
  )
}

interface TrayRowProps {
  entry: RemovalEntry
  onCancel: (entryId: string) => void
  onCommit: (entry: RemovalEntry) => void
}

function TrayRow({ entry, onCancel, onCommit }: TrayRowProps) {
  const { deadlineAt } = entry

  return (
    <div
      data-removal-entry={entry.entryId}
      className={cn(ROW_BASE, ROW_INACTIVE, 'relative cursor-default')}
    >
      <TrayGlyph entry={entry} />
      {/* Same face the row wears in the tree: a branch is a git ref and reads in
          mono, a folder name is prose and does not. A row that changed typeface
          on its way into the tray would read as a different kind of thing at the
          one moment the user is deciding whether to keep it. */}
      <span
        className={cn(
          'min-w-0 flex-1 truncate text-left',
          entry.kind === 'folder' ? 'font-sans font-semibold tracking-[0.005em]' : 'font-mono',
        )}
      >
        {entry.label}
      </span>
      {entry.extra > 0 && <GoesWith n={entry.extra} />}

      {deadlineAt === null ? (
        // A repo takes every worktree under it, so it asks rather than counts
        // down: two real controls, and nothing happens until one is pressed.
        <>
          <button
            type="button"
            className="shrink-0 rounded-md px-2 py-1 text-[11px] font-semibold text-muted-foreground hover:bg-sidebar-element-hover hover:text-foreground"
            onClick={() => onCancel(entry.entryId)}
          >
            Cancel
          </button>
          <button
            type="button"
            className="shrink-0 rounded-md px-2 py-1 text-[11px] font-semibold text-destructive hover:bg-destructive/10"
            onClick={() => onCommit(entry)}
          >
            Remove
          </button>
        </>
      ) : (
        <>
          {/* The same clock the hairline draws, said in figures. The bar answers
              "roughly how long"; a row that is about to take a worktree with it
              deserves to answer "how long" exactly. Rendered from the deadline
              rather than from a tick, so a row that re-renders for some other
              reason still shows the truth; the counting itself happens in the
              tray's clock below and never comes back through React. */}
          <span
            data-removal-secs={entry.entryId}
            className="shrink-0 text-[11px] tabular-nums text-muted-foreground"
          >
            {secondsLeft(deadlineAt, Date.now())}
          </span>
          <button
            type="button"
            className={ROW_SUB_ACTION}
            aria-label={`Keep ${entry.label}`}
            title="Keep"
            onClick={() => onCancel(entry.entryId)}
          >
            <svg
              aria-hidden="true"
              className="size-3"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M6 3L2.5 6.5 6 10M2.5 6.5H10a3.5 3.5 0 0 1 0 7H7" />
            </svg>
          </button>
          {/* The hairline, drawn once. `data-essential-motion` keeps its eight
              seconds under prefers-reduced-motion: this bar is not decoration,
              it is how long is left before something is deleted. */}
          <span
            data-removal-drain=""
            data-essential-motion=""
            className="pointer-events-none absolute inset-x-2 bottom-[3px] h-0.5 origin-left rounded-full bg-destructive animate-tray-drain"
          />
        </>
      )}
    </div>
  )
}

export function RemovalTray() {
  const entries = useRemovalTrayStore((s) => s.entries)
  const navigate = useNavigate()
  const router = useRouter()
  // The rows the clock below writes into. Scoped to the tray so the one query
  // it makes per second cannot wander into the rest of the sidebar.
  const listRef = useRef<HTMLDivElement | null>(null)

  // Kept on a ref so the scheduling effect below is driven by the entry list and
  // nothing else — a router that hands out a fresh function every render would
  // otherwise rebuild every clock on every navigation.
  const commitRef = useRef<(entry: RemovalEntry) => void>(() => {})
  commitRef.current = (entry) => {
    const go: RemovalNavigate = (target) => {
      if (!target) void navigate({ to: '/' })
      else void navigate({ to: '/ide/$projectId/$repoId/$wsId', params: target })
    }
    void commitRemoval(entry, {
      activeWorkspaceId:
        router.state.location.pathname.match(/\/ide\/[^/]+\/[^/]+\/([^/]+)/)?.[1] ?? '',
      navigate: go,
    })
  }

  const commit = useCallback((entry: RemovalEntry) => commitRef.current(entry), [])

  const cancel = useCallback((entryId: string) => {
    useRemovalTrayStore.getState().cancel(entryId)
  }, [])

  // ONE timer for the whole tray, aimed at whichever deadline comes first.
  //
  // Every held row already draws its own eight seconds as a CSS animation, so
  // nothing here needs to tick — the timer's only job is to fire once, at the
  // instant the first hairline runs out. Committing that row shortens the list,
  // which re-runs this effect and re-aims at the next one, so N rows still cost
  // exactly one live timer.
  //
  // `deadlineAt` is a wall-clock instant rather than a duration, which is what
  // makes re-aiming free: a timer rebuilt halfway through resumes where the
  // drain already is instead of starting another eight seconds.
  useEffect(() => {
    let soonest: number | null = null
    for (const entry of entries) {
      if (entry.deadlineAt === null) continue
      if (soonest === null || entry.deadlineAt < soonest) soonest = entry.deadlineAt
    }
    if (soonest === null) return
    const timer = setTimeout(
      () => {
        const now = Date.now()
        for (const entry of useRemovalTrayStore.getState().entries) {
          if (entry.deadlineAt !== null && entry.deadlineAt <= now) commit(entry)
        }
      },
      Math.max(0, soonest - Date.now()),
    )
    return () => clearTimeout(timer)
  }, [entries, commit])

  // The seconds numerals, written straight to the DOM.
  //
  // One `textContent` write per held row, once a second, on a timer that belongs
  // to the tray rather than to any row — so the figures count without a single
  // render. Through state this would repaint every row in the tray to change one
  // digit, which is the cost the hairline was made a CSS animation to avoid.
  //
  // The wake-up is aimed at the next whole second of whichever row crosses one
  // first, so the numeral changes exactly when its value does and the tray is
  // asleep in between. Reduced motion has nothing to switch off here: a number
  // counting down is information, not animation.
  useEffect(() => {
    const deadlines = new Map<string, number>()
    for (const entry of entries) {
      if (entry.deadlineAt !== null) deadlines.set(entry.entryId, entry.deadlineAt)
    }
    const list = listRef.current
    if (!list || deadlines.size === 0) return

    let timer = 0
    const tick = () => {
      const now = Date.now()
      let delay = 1000
      for (const el of list.querySelectorAll<HTMLElement>('[data-removal-secs]')) {
        const deadlineAt = deadlines.get(el.dataset.removalSecs ?? '')
        if (deadlineAt === undefined) continue
        const secs = String(secondsLeft(deadlineAt, now))
        // Written only where it differs, so a tray of rows on the same deadline
        // is one comparison each and no DOM work at all until the digit turns.
        if (el.textContent !== secs) el.textContent = secs
        const left = deadlineAt - now
        if (left > 0) delay = Math.min(delay, ((left - 1) % 1000) + 1)
      }
      timer = window.setTimeout(tick, delay)
    }
    // The figures are already right — this render put them there. Running the
    // tick now is what aims the first wake-up at a second boundary rather than
    // an arbitrary offset from whenever the row happened to be held.
    tick()
    return () => clearTimeout(timer)
  }, [entries])

  if (entries.length === 0) return null

  return (
    <div className="shrink-0 border-t border-border bg-background/40 pt-1 pb-1.5">
      <div className="px-3 pt-1 pb-0.5 text-[10px] uppercase tracking-[0.06em] text-muted-foreground">
        {entries.some((e) => e.deadlineAt === null) ? 'Waiting on you' : 'Removing'}
      </div>
      <div ref={listRef}>
        {entries.map((entry) => (
          <TrayRow key={entry.entryId} entry={entry} onCancel={cancel} onCommit={commit} />
        ))}
      </div>
    </div>
  )
}
