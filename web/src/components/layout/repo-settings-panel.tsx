import { useState, useEffect } from 'react'
import { Sheet, SheetPopup, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { apiFetch } from '@/lib/api'
import { Lock, GitBranch } from 'lucide-react'
import { cn } from '@/lib/utils'

interface BranchEntry {
  name: string
  isProtected: boolean
  hasWorkspace: boolean
}

interface RepoSettingsPanelProps {
  repoId: string
  repoName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function RepoSettingsPanel({ repoId, repoName, open, onOpenChange }: RepoSettingsPanelProps) {
  const [branches, setBranches] = useState<BranchEntry[]>([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [importing, setImporting] = useState(false)

  useEffect(() => {
    if (!open) return
    setBranches([])
    setSelected(new Set())
    setFilter('')
    apiFetch<BranchEntry[]>(`/v0/repos/${repoId}/branches`)
      .then(setBranches)
      .catch(() => {})
  }, [open, repoId])

  const visible = branches.filter((b) =>
    b.name.toLowerCase().includes(filter.toLowerCase())
  )

  const importable = selected.size

  async function handleImport() {
    if (selected.size === 0) return
    setImporting(true)
    await Promise.all(
      Array.from(selected).map((branch) =>
        apiFetch('/v0/workspaces', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ repoId, branch }),
        }).catch(() => {})
      )
    )
    setImporting(false)
    onOpenChange(false)
  }

  function toggleBranch(name: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetPopup side="left" className="w-80">
        <SheetHeader>
          <SheetTitle className="font-mono text-sm">{repoName} — Settings</SheetTitle>
        </SheetHeader>

        <div className="flex flex-col gap-4 p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Import Workspaces
          </h3>

          <Input
            placeholder="Filter branches…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="h-7 text-xs"
          />

          <div className="flex flex-col gap-0.5">
            {visible.some((b) => b.isProtected) && (
              <>
                <p className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Protected — auto-imported
                </p>
                {visible.filter((b) => b.isProtected).map((b) => (
                  <label
                    key={b.name}
                    className="flex cursor-default items-center gap-2 rounded px-2 py-1.5 text-xs opacity-60"
                  >
                    <Checkbox checked={true} disabled={true} ariaLabel={b.name} />
                    <Lock className="size-3 shrink-0 text-muted-foreground" />
                    <span className="min-w-0 flex-1 truncate font-mono">{b.name}</span>
                  </label>
                ))}
              </>
            )}

            {visible.some((b) => !b.isProtected) && (
              <>
                <p className="mb-1 mt-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Other branches
                </p>
                {visible.filter((b) => !b.isProtected).map((b) => (
                  <label
                    key={b.name}
                    className={cn(
                      'flex items-center gap-2 rounded px-2 py-1.5 text-xs hover:bg-accent',
                      b.hasWorkspace ? 'cursor-default opacity-60' : 'cursor-pointer',
                    )}
                  >
                    {!b.hasWorkspace ? (
                      <Checkbox
                        checked={selected.has(b.name)}
                        onChange={() => toggleBranch(b.name)}
                        ariaLabel={b.name}
                      />
                    ) : (
                      <Checkbox checked={true} disabled={true} ariaLabel={b.name} />
                    )}
                    <GitBranch className="size-3 shrink-0 text-muted-foreground" />
                    <span className="min-w-0 flex-1 truncate font-mono">{b.name}</span>
                    {b.hasWorkspace && (
                      <span className="shrink-0 text-[10px] text-green-500">imported</span>
                    )}
                  </label>
                ))}
              </>
            )}
          </div>

          <Button
            size="sm"
            disabled={selected.size === 0 || importing}
            onClick={handleImport}
          >
            {importing ? 'Importing…' : importable > 0 ? `Import ${importable} branch${importable > 1 ? 'es' : ''}` : 'Import'}
          </Button>
        </div>
      </SheetPopup>
    </Sheet>
  )
}
