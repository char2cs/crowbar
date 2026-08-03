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
  /** Fires the import for the selected branches. The tree owns the optimistic
   *  loading rows + the batch API call, so the dialog just closes afterwards. */
  onImport: (branches: string[]) => void
}

/**
 * Branch-import modal: a virtualized, searchable list of the repo's remote
 * branches with a live "creates N parents" hint. Fetches branches (fast, local
 * git) and the open-PR graph (slower, network) in parallel on open — the list
 * renders immediately and the graph only enriches the hint. Import posts the
 * whole selection in one batch; the daemon PR-parents and creates missing
 * ancestors server-side (the client hint is advisory).
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
  const [prLinks, setPrLinks] = useState<PRLink[]>([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const scrollRef = useRef<HTMLDivElement>(null)

  // Fetch on open; the branch list and PR graph load independently so the list
  // never waits on the network call.
  useEffect(() => {
    if (!open) return
    setSelected(new Set())
    setFilter('')
    setPrLinks([])
    apiFetch<BranchEntry[]>(`${repoBase}/branches`)
      .then(setBranches)
      .catch(() => setBranches([]))
    getRepoPullRequests(projectId, repoId)
      .then(setPrLinks)
      .catch(() => setPrLinks([]))
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
  }

  function handleImport() {
    if (selected.size === 0) return
    // Hand off to the tree, which drops optimistic loading rows in immediately
    // and fires the batch import; close the modal so that feedback is visible.
    onImport([...selected])
    onOpenChange(false)
  }

  const hint =
    plan.parentCount > 0
      ? `Imports ${plan.importCount} branch${plan.importCount === 1 ? '' : 'es'} · creates ${plan.parentCount} parent branch${plan.parentCount === 1 ? '' : 'es'}`
      : `Imports ${plan.importCount} branch${plan.importCount === 1 ? '' : 'es'}`

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup
        className="flex h-[70vh] max-w-md flex-col p-0"
        data-oracle-id="repo-import-dialog-popup"
      >
        <DialogHeader className="p-4 pb-2" data-oracle-id="repo-import-dialog-header">
          <DialogTitle data-oracle-id="repo-import-dialog-title">Import branches</DialogTitle>
          <DialogDescription data-oracle-id="repo-import-dialog-description">
            Bring remote branches into Crowbar as workspaces.
          </DialogDescription>
        </DialogHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-2 px-4 pb-4">
          <Input
            placeholder="Search branches…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="h-7 shrink-0 text-xs"
          />
          <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto">
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
                    <ImportRow branch={b} checked={selected.has(b.name)} onToggle={toggle} />
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
}: {
  branch: BranchEntry
  checked: boolean
  onToggle: (name: string) => void
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
    <label className="flex h-full cursor-pointer items-center gap-2 rounded px-1 text-xs hover:bg-accent/60">
      <Checkbox checked={checked} onChange={() => onToggle(branch.name)} ariaLabel={branch.name} />
      <span className="min-w-0 flex-1 truncate font-mono">{branch.name}</span>
    </label>
  )
}
