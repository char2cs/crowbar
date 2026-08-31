import { useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Lock, Check } from 'lucide-react'
import {
  Dialog,
  DialogPopup,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { apiFetch, getRepoPullRequests } from '@/lib/api'
import { computeImportPlan, type BranchEntry, type PRLink } from '@/lib/import/parent-plan'

const IMPORT_ROW_HEIGHT = 30

interface RepoImportDialogProps {
  projectId: string
  repoId: string
  defaultBranch: string
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Fires the import for the selected branches, plus which of THOSE branches
   *  the user chose to lock (a subset of `branches`, empty when none were
   *  chosen — importing with no lock choice behaves exactly as it always has).
   *  The tree owns the optimistic loading rows + the batch API call, so the
   *  dialog just closes afterwards. */
  onImport: (branches: string[], lockedBranches: string[]) => void
}

/**
 * Branch-import modal: a virtualized, searchable list of the repo's remote
 * branches with a live "creates N parents" hint. Fetches branches and the
 * open-PR graph in parallel on open; both are network calls (the branch list
 * refreshes origin server-side before listing, so it reports the remote as it
 * is now rather than as the clone last heard it), so the list carries a real
 * loading state instead of rendering as momentarily empty. Import posts the
 * whole selection in one batch; the daemon PR-parents and creates missing
 * ancestors server-side (the client hint is advisory).
 *
 * A checked row also carries its own lock toggle (Task 6): each branch chooses
 * its post-import locked state right here, rather than requiring a separate
 * Lock action from the row context menu afterwards. The import call itself
 * only 202s — no workspace id exists yet — so the caller locks each branch
 * once its own workspace actually lands (see `performImportBranches`).
 */
export function RepoImportDialog({
  projectId,
  repoId,
  defaultBranch,
  open,
  onOpenChange,
  onImport,
}: RepoImportDialogProps) {
  const repoBase = `/v0/projects/${projectId}/repos/${repoId}`
  const [branches, setBranches] = useState<BranchEntry[]>([])
  const [loadState, setLoadState] = useState<'loading' | 'ready' | 'failed'>('loading')
  const [prLinks, setPrLinks] = useState<PRLink[]>([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  // Per-branch lock choice (Task 6): which of the SELECTED branches should
  // come in locked. `toggle` clears a row's entry here when it deselects it,
  // so this is always a subset of `selected`; handleImport still intersects
  // with `selected` as a belt-and-suspenders guard.
  const [lockedBranches, setLockedBranches] = useState<Set<string>>(new Set())
  const scrollRef = useRef<HTMLDivElement>(null)

  // Fetch on open; the branch list and PR graph load independently so the hint
  // never holds the list back. A reopen re-fetches rather than showing the
  // previous open's branches — the whole point of the server-side refresh is
  // that the remote may have moved since.
  //
  // react-doctor-disable-next-line no-reset-all-state-on-prop-change -- the harm the rule names ("users briefly see stale state") cannot happen here: `loadState` is reset to 'loading' in this same synchronous body and is what gates the list's render below, so a reopen shows "Fetching branches from the remote…" and never the previous open's branches. The prescribed fix — mount/key the body on `open` — would unmount it during the Dialog's exit animation, leaving an empty popup for the length of the close.
  useEffect(() => {
    if (!open) return
    setSelected(new Set())
    setLockedBranches(new Set())
    setFilter('')
    setPrLinks([])
    setBranches([])
    setLoadState('loading')
    let live = true
    apiFetch<BranchEntry[]>(`${repoBase}/branches`)
      .then((list) => {
        if (!live) return
        setBranches(list)
        setLoadState('ready')
      })
      .catch(() => {
        if (!live) return
        setBranches([])
        setLoadState('failed')
      })
    getRepoPullRequests(projectId, repoId)
      .then((links) => live && setPrLinks(links))
      .catch(() => live && setPrLinks([]))
    return () => {
      live = false
    }
  }, [open, repoBase, projectId, repoId])

  const visible = useMemo(() => {
    const q = filter.toLowerCase()
    const list = branches.filter((b) => b.name.toLowerCase().includes(q))
    // Protected first (locked, non-importable), then the rest.
    return [...list.filter((b) => b.isProtected), ...list.filter((b) => !b.isProtected)]
  }, [branches, filter])

  const rowVirtualizer = useVirtualizer({
    count: visible.length,
    estimateSize: () => IMPORT_ROW_HEIGHT,
    getScrollElement: () => scrollRef.current,
    overscan: 10,
    // Read the viewport synchronously on attach (the default observer relies on
    // ResizeObserver alone, which measures late — and never in jsdom); keep it
    // updated on resize thereafter.
    observeElementRect: (instance, cb) => {
      const el = instance.scrollElement
      if (!el) return
      const r = el.getBoundingClientRect()
      cb({ width: Math.round(r.width), height: Math.round(r.height) })
      const ro = new ResizeObserver(([entry]) => {
        const { width, height } = entry.contentRect
        cb({ width: Math.round(width), height: Math.round(height) })
      })
      ro.observe(el)
      return () => ro.disconnect()
    },
  })

  const plan = useMemo(
    () => computeImportPlan([...selected], prLinks, branches, defaultBranch),
    [selected, prLinks, branches, defaultBranch],
  )

  function toggle(name: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
    // Deselecting a row also drops its lock choice — reselecting it later
    // starts fresh rather than silently reapplying a choice made earlier in
    // the same open.
    setLockedBranches((prev) => {
      if (!prev.has(name)) return prev
      const next = new Set(prev)
      next.delete(name)
      return next
    })
  }

  function toggleLock(name: string) {
    setLockedBranches((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  function handleImport() {
    if (selected.size === 0) return
    // Hand off to the tree, which drops optimistic loading rows in immediately
    // and fires the batch import; close the modal so that feedback is visible.
    // Lock choice is filtered down to branches actually being imported — a
    // deselected branch's stale lock choice must never leak into the call.
    onImport(
      [...selected],
      [...lockedBranches].filter((b) => selected.has(b)),
    )
    onOpenChange(false)
  }

  // Shown in place of the (virtualized) rows whenever there is nothing to show:
  // the fetch failed, the remote has no branches, or the filter matched none.
  const emptyMessage =
    loadState === 'failed'
      ? "Couldn't reach the remote to list branches."
      : branches.length === 0
        ? 'No branches on the remote.'
        : 'No branches match that search.'

  const hint =
    plan.parentCount > 0
      ? `Imports ${plan.importCount} branch${plan.importCount === 1 ? '' : 'es'} · creates ${plan.parentCount} parent branch${plan.parentCount === 1 ? '' : 'es'}`
      : `Imports ${plan.importCount} branch${plan.importCount === 1 ? '' : 'es'}`

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup className="flex h-[70vh] max-w-md flex-col p-0">
        <DialogHeader className="p-4 pb-2">
          <DialogTitle>Import branches</DialogTitle>
          <DialogDescription>Bring remote branches into Crowbar as workspaces.</DialogDescription>
        </DialogHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-2 px-4 pb-4">
          <Input
            placeholder="Search branches…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="h-7 shrink-0 text-xs"
          />
          <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto">
            {visible.length === 0 && (
              <p className="px-1 py-2 text-xs text-muted-foreground">
                {loadState === 'loading' ? 'Fetching branches from the remote…' : emptyMessage}
              </p>
            )}
            <div
              style={{ height: rowVirtualizer.getTotalSize(), position: 'relative', width: '100%' }}
            >
              {rowVirtualizer.getVirtualItems().map((vi) => {
                const b = visible[vi.index]
                return (
                  <div
                    key={b.name}
                    style={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      width: '100%',
                      height: vi.size,
                      transform: `translateY(${vi.start}px)`,
                    }}
                  >
                    <ImportRow
                      branch={b}
                      checked={selected.has(b.name)}
                      onToggle={toggle}
                      locked={lockedBranches.has(b.name)}
                      onToggleLock={toggleLock}
                    />
                  </div>
                )
              })}
            </div>
          </div>
          <p className="shrink-0 text-xs text-muted-foreground">{hint}</p>
          <Button
            size="sm"
            className="shrink-0"
            disabled={selected.size === 0}
            onClick={handleImport}
          >
            Import
          </Button>
        </div>
      </DialogPopup>
    </Dialog>
  )
}

function ImportRow({
  branch,
  checked,
  onToggle,
  locked,
  onToggleLock,
}: {
  branch: BranchEntry
  checked: boolean
  onToggle: (name: string) => void
  locked: boolean
  onToggleLock: (name: string) => void
}) {
  if (branch.isProtected) {
    return (
      <div className="flex h-full items-center gap-2 px-1 opacity-40">
        <Lock className="size-3 shrink-0" />
        <span className="min-w-0 flex-1 truncate font-mono text-xs">{branch.name}</span>
      </div>
    )
  }
  if (branch.hasWorkspace) {
    return (
      <div className="flex h-full items-center gap-2 px-1 opacity-40">
        <Check className="size-3 shrink-0 text-green-500" />
        <span className="min-w-0 flex-1 truncate font-mono text-xs">{branch.name}</span>
      </div>
    )
  }
  return (
    <div className="flex h-full items-center gap-2 rounded px-1 text-xs hover:bg-accent/60">
      <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-2">
        <Checkbox checked={checked} onChange={() => onToggle(branch.name)} ariaLabel={branch.name} />
        <span className="min-w-0 flex-1 truncate font-mono">{branch.name}</span>
      </label>
      {/* Lock choice only matters — and only shows — once the branch is
       *  actually part of the import; the smallest addition the brief asked
       *  for, not a second control competing for attention on every row. */}
      {checked && (
        <button
          type="button"
          aria-pressed={locked}
          aria-label={locked ? `Don't lock ${branch.name} after import` : `Lock ${branch.name} after import`}
          onClick={() => onToggleLock(branch.name)}
          className={`shrink-0 rounded p-0.5 ${locked ? 'text-foreground' : 'text-muted-foreground/40 hover:text-foreground'}`}
        >
          <Lock className="size-3" />
        </button>
      )}
    </div>
  )
}
