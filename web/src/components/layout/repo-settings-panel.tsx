import { useState, useEffect, useRef } from 'react'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { apiFetch } from '@/lib/api'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useSidebarStore } from '@/lib/store/sidebar'
import { Lock, GitBranch, Trash2 } from 'lucide-react'
import { cn } from '@/lib/utils'

interface BranchEntry {
  name: string
  isProtected: boolean
  hasWorkspace: boolean
}

interface RepoSettingsPanelProps {
  repoId: string
  repoName: string
}

export function RepoSettingsPanel({ repoId, repoName }: RepoSettingsPanelProps) {
  const [branches, setBranches] = useState<BranchEntry[]>([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [importing, setImporting] = useState(false)
  const [emojiInput, setEmojiInput] = useState('')
  const [iconLoading, setIconLoading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  const repo = useSidebarStore((s) => s.repos.find((r) => r.id === repoId))

  useEffect(() => {
    setBranches([])
    setSelected(new Set())
    setFilter('')
    apiFetch<BranchEntry[]>(`/v0/repos/${repoId}/branches`)
      .then(setBranches)
      .catch(() => {})
  }, [repoId])

  const visible = branches.filter((b) =>
    b.name.toLowerCase().includes(filter.toLowerCase())
  )

  async function handleImport() {
    if (selected.size === 0) return
    setImporting(true)
    try {
      await Promise.all(
        Array.from(selected).map((branch) =>
          apiFetch('/v0/workspaces', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ repoId, branch }),
          }).catch(() => {})
        )
      )
      void useWorkspaceListStore.getState().fetch()
    } finally {
      setImporting(false)
    }
  }

  function toggleBranch(name: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    setIconLoading(true)
    try {
      const form = new FormData()
      form.append('icon', file)
      await apiFetch(`/v0/repos/${repoId}/icon`, { method: 'PUT', body: form })
      void useWorkspaceListStore.getState().fetch()
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  async function handleEmojiSubmit() {
    const emoji = emojiInput.trim()
    if (!emoji) return
    setIconLoading(true)
    try {
      await apiFetch(`/v0/repos/${repoId}/icon/emoji`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emoji }),
      })
      setEmojiInput('')
      void useWorkspaceListStore.getState().fetch()
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
    }
  }

  async function handleGithubAvatar() {
    setIconLoading(true)
    try {
      await apiFetch(`/v0/repos/${repoId}/icon/github`, { method: 'PUT' })
      void useWorkspaceListStore.getState().fetch()
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
    }
  }

  async function handleResetIcon() {
    setIconLoading(true)
    try {
      await apiFetch(`/v0/repos/${repoId}/icon`, { method: 'DELETE' })
      void useWorkspaceListStore.getState().fetch()
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
    }
  }

  const importable = selected.size

  return (
    <ScrollArea className="flex-1">
      <div className="flex flex-col gap-4 p-3">

        {/* Icon section */}
        <div className="flex flex-col gap-2">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            Icon
          </p>

          <div className="flex items-center gap-3 rounded-md border border-border bg-accent/30 p-2.5">
            {repo?.avatarURL?.startsWith('emoji:') ? (
              <span className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-md text-2xl">
                {repo.avatarURL.slice(6)}
              </span>
            ) : repo?.avatarURL ? (
              <img
                src={repo.avatarURL}
                alt={repoName}
                className="h-9 w-9 flex-shrink-0 rounded-md object-cover"
              />
            ) : (
              <span
                className={cn(
                  'flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-md text-sm font-bold text-primary-foreground',
                  repo?.avatarColor,
                )}
              >
                {repo?.avatarLabel}
              </span>
            )}
            <div className="flex flex-col gap-1.5 flex-1">
              <input
                ref={fileRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                className="hidden"
                onChange={handleFileChange}
              />
              <button
                onClick={() => fileRef.current?.click()}
                disabled={iconLoading}
                className="text-left text-[10.5px] text-muted-foreground hover:text-foreground disabled:opacity-50"
              >
                📁 Upload image
              </button>
              <div className="flex items-center gap-1.5">
                <input
                  value={emojiInput}
                  onChange={(e) => setEmojiInput(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter') void handleEmojiSubmit() }}
                  placeholder="😀 Type emoji…"
                  maxLength={4}
                  className="h-6 w-24 rounded border border-border bg-background px-1.5 text-[10.5px] outline-none focus:border-ring"
                />
                {emojiInput && (
                  <button
                    onClick={() => void handleEmojiSubmit()}
                    disabled={iconLoading}
                    className="text-[10px] text-muted-foreground hover:text-foreground"
                  >
                    Set
                  </button>
                )}
              </div>
              <button
                onClick={() => void handleGithubAvatar()}
                disabled={iconLoading}
                className="text-left text-[10.5px] text-muted-foreground hover:text-foreground disabled:opacity-50"
              >
                🐙 Use GitHub avatar
              </button>
            </div>
            {repo?.avatarURL && (
              <button
                onClick={() => void handleResetIcon()}
                disabled={iconLoading}
                aria-label="Reset icon"
                className="flex-shrink-0 text-muted-foreground/50 hover:text-destructive"
              >
                <Trash2 className="size-3" />
              </button>
            )}
          </div>
        </div>

        {/* Branch import section */}
        <div className="flex flex-col gap-2">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            Import Workspaces
          </p>

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
            {importing
              ? 'Importing…'
              : importable > 0
              ? `Import ${importable} branch${importable > 1 ? 'es' : ''}`
              : 'Import'}
          </Button>
        </div>

      </div>
    </ScrollArea>
  )
}
