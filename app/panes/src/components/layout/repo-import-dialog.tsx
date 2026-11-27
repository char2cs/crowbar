import { useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { CheckIcon, LockIcon } from '@phosphor-icons/react'
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
import { apiFetch } from '@/lib/api/fetch'
import { getRepoPullRequests } from '@/lib/api'
import { useHostResolved } from '@/lib/host-resolve'
import { repoBase } from '@/lib/project-url'
import { computeImportPlan, type BranchEntry, type PRLink } from '@/lib/import/parent-plan'

const IMPORT_ROW_HEIGHT = 30

interface RepoImportDialogProps {
  projectId: string
  repoId: string
  defaultBranch: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onImport: (branches: string[], lockedBranches: string[]) => void
}

export function RepoImportDialog({
  projectId,
  repoId,
  defaultBranch,
  open,
  onOpenChange,
  onImport,
}: RepoImportDialogProps) {
  const hostResolved = useHostResolved({ projectId })
  const [branches, setBranches] = useState<BranchEntry[]>([])
  const [loadState, setLoadState] = useState<'loading' | 'ready' | 'failed'>('loading')
  const [prLinks, setPrLinks] = useState<PRLink[]>([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [lockedBranches, setLockedBranches] = useState<Set<string>>(new Set())
  const scrollRef = useRef<HTMLDivElement>(null)

  // react-doctor-disable-next-line no-reset-all-state-on-prop-change -- the harm the rule names ("users briefly see stale state") cannot happen here: `loadState` is reset to 'loading' in this same synchronous body and is what gates the list's render below, so a reopen shows "Fetching branches from the remote…" and never the previous open's branches. The prescribed fix — mount/key the body on `open` — would unmount it during the Dialog's exit animation, leaving an empty popup for the length of the close.
  useEffect(() => {
    if (!open || !hostResolved) return
    setSelected(new Set())
    setLockedBranches(new Set())
    setFilter('')
    setPrLinks([])
    setBranches([])
    setLoadState('loading')
    let live = true
    apiFetch<BranchEntry[]>(`${repoBase(projectId, repoId)}/branches`)
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
  }, [open, hostResolved, projectId, repoId])

  const visible = useMemo(() => {
    const q = filter.toLowerCase()
    const list = branches.filter((b) => b.name.toLowerCase().includes(q))
    return [...list.filter((b) => b.isProtected), ...list.filter((b) => !b.isProtected)]
  }, [branches, filter])

  const rowVirtualizer = useVirtualizer({
    count: visible.length,
    estimateSize: () => IMPORT_ROW_HEIGHT,
    getScrollElement: () => scrollRef.current,
    overscan: 10,
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
    onImport(
      [...selected],
      [...lockedBranches].filter((b) => selected.has(b)),
    )
    onOpenChange(false)
  }

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
        <LockIcon className="size-3 shrink-0" />
        <span className="min-w-0 flex-1 truncate font-mono text-xs">{branch.name}</span>
      </div>
    )
  }
  if (branch.hasWorkspace) {
    return (
      <div className="flex h-full items-center gap-2 px-1 opacity-40">
        <CheckIcon className="size-3 shrink-0 text-green-500" />
        <span className="min-w-0 flex-1 truncate font-mono text-xs">{branch.name}</span>
      </div>
    )
  }
  return (
    <div className="flex h-full items-center gap-2 rounded px-1 text-xs hover:bg-accent/60">
      <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-2">
        <Checkbox
          checked={checked}
          onChange={() => onToggle(branch.name)}
          ariaLabel={branch.name}
        />
        <span className="min-w-0 flex-1 truncate font-mono">{branch.name}</span>
      </label>
      {checked && (
        <button
          type="button"
          aria-pressed={locked}
          aria-label={
            locked ? `Don't lock ${branch.name} after import` : `Lock ${branch.name} after import`
          }
          onClick={() => onToggleLock(branch.name)}
          className={`shrink-0 rounded p-0.5 ${locked ? 'text-foreground' : 'text-muted-foreground/40 hover:text-foreground'}`}
        >
          <LockIcon className="size-3" />
        </button>
      )}
    </div>
  )
}
