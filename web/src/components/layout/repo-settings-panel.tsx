import { useState, useEffect, useRef } from 'react'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'
import { apiFetch, postWorkspace } from '@/lib/api'
import { useSidebarStore } from '@/lib/store/sidebar'
import { toast } from 'sonner'
import { Lock, Check, Trash2, Upload, Star, Smile } from 'lucide-react'
import { cn } from '@/lib/utils'

interface BranchEntry {
  name: string
  isProtected: boolean
  hasWorkspace: boolean
}

interface RepoSettingsPanelProps {
  projectId: string
  repoId: string
  repoName: string
}

// §3: every repo-scoped route is hierarchical under the owning project now.
export function RepoSettingsPanel({ projectId, repoId, repoName }: RepoSettingsPanelProps) {
  const repoBase = `/v0/projects/${projectId}/repos/${repoId}`
  const [branches, setBranches] = useState<BranchEntry[]>([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [importing, setImporting] = useState(false)
  const [emojiInput, setEmojiInput] = useState('')
  const [showEmojiInput, setShowEmojiInput] = useState(false)
  const [iconLoading, setIconLoading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  const repo = useSidebarStore((s) => s.repos.find((r) => r.id === repoId))

  useEffect(() => {
    setBranches([])
    setSelected(new Set())
    setFilter('')
    apiFetch<BranchEntry[]>(`${repoBase}/branches`)
      .then(setBranches)
      .catch(() => {})
  }, [repoBase])

  const visible = branches.filter((b) =>
    b.name.toLowerCase().includes(filter.toLowerCase())
  )

  async function handleImport() {
    if (selected.size === 0) return
    setImporting(true)
    try {
      // §3/§4: each branch import is a hierarchical 202 POST. The imported
      // workspaces arrive on the per-repo WS stream and the WS-driven cache
      // inserts the rows — no post-mutation list refetch/merge. We only refresh
      // the branch list here so the dialog reflects hasWorkspace=true.
      const results = await Promise.allSettled(
        Array.from(selected).map((branch) => postWorkspace(projectId, repoId, branch)),
      )
      const failed = results.filter((r) => r.status === 'rejected')
      if (failed.length > 0) {
        const msg = failed[0].status === 'rejected'
          ? String((failed[0] as PromiseRejectedResult).reason)
          : 'Unknown error'
        toast.error(`Failed to import ${failed.length} branch${failed.length > 1 ? 'es' : ''}: ${msg}`)
      }
      apiFetch<BranchEntry[]>(`${repoBase}/branches`)
        .then(setBranches)
        .catch(() => {})
      setSelected(new Set())
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
      // §3: the updated RepoDTO (new avatarUrl) arrives on the repos WS stream
      // and merges into the cache — no manual list refetch.
      await apiFetch(`${repoBase}/icon`, { method: 'PUT', body: form })
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
      await apiFetch(`${repoBase}/icon/emoji`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emoji }),
      })
      setEmojiInput('')
      setShowEmojiInput(false)
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
    }
  }

  async function handleGithubAvatar() {
    setIconLoading(true)
    try {
      await apiFetch(`${repoBase}/icon/github`, { method: 'PUT' })
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
    }
  }

  async function handleResetIcon() {
    setIconLoading(true)
    try {
      await apiFetch(`${repoBase}/icon`, { method: 'DELETE' })
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
    }
  }

  const importable = selected.size
  const isEmoji = repo?.avatarURL?.startsWith('emoji:')
  const avatarSrc = !isEmoji && repo?.avatarURL ? repo.avatarURL : undefined

  return (
    <div className="flex h-full flex-col">

      {/* Icon section — fixed at top */}
      <div className="flex-shrink-0 border-b border-border p-3 flex flex-col gap-3">
        <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          Icon
        </p>

        <div className="flex justify-center">
          <Avatar className="size-14 rounded-xl text-base">
            {isEmoji ? (
              <AvatarFallback className="rounded-xl bg-transparent text-2xl">
                {repo!.avatarURL!.slice(6)}
              </AvatarFallback>
            ) : avatarSrc ? (
              <AvatarImage src={avatarSrc} alt={repoName} />
            ) : (
              <AvatarFallback className={cn('rounded-xl text-sm font-bold text-primary-foreground', repo?.avatarColor)}>
                {repo?.avatarLabel}
              </AvatarFallback>
            )}
          </Avatar>
        </div>

        <div className="flex gap-1.5">
          <input
            ref={fileRef}
            type="file"
            accept="image/png,image/jpeg,image/webp"
            className="hidden"
            onChange={handleFileChange}
          />
          <Button
            variant="ghost"
            size="xs"
            disabled={iconLoading}
            onClick={() => { setShowEmojiInput(false); fileRef.current?.click() }}
            className="flex-1 gap-1 text-muted-foreground hover:text-foreground"
          >
            <Upload className="size-3" />
            Upload
          </Button>
          <Button
            variant={showEmojiInput ? 'secondary' : 'ghost'}
            size="xs"
            disabled={iconLoading}
            onClick={() => setShowEmojiInput((v) => !v)}
            className="flex-1 gap-1 text-muted-foreground hover:text-foreground"
          >
            <Smile className="size-3" />
            Emoji
          </Button>
          <Button
            variant="ghost"
            size="xs"
            disabled={iconLoading}
            onClick={() => { setShowEmojiInput(false); void handleGithubAvatar() }}
            className="flex-1 gap-1 text-muted-foreground hover:text-foreground"
          >
            <Star className="size-3" />
            GitHub
          </Button>
        </div>

        {showEmojiInput && (
          <div className="flex gap-1.5">
            <Input
              value={emojiInput}
              onChange={(e) => setEmojiInput(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') void handleEmojiSubmit() }}
              placeholder="Type an emoji…"
              maxLength={4}
              className="h-7 flex-1 text-center text-base"
              autoFocus
            />
            <Button
              size="sm"
              className="h-7"
              disabled={!emojiInput.trim() || iconLoading}
              onClick={() => void handleEmojiSubmit()}
            >
              Set
            </Button>
          </div>
        )}

        {repo?.avatarURL && (
          <Button
            variant="ghost"
            size="xs"
            disabled={iconLoading}
            onClick={() => void handleResetIcon()}
            className="w-full gap-1 text-muted-foreground/60 hover:text-destructive"
          >
            <Trash2 className="size-3" />
            Reset to default
          </Button>
        )}
      </div>

      {/* Branch import section — fills remaining height */}
      <div className="flex min-h-0 flex-1 flex-col gap-2 p-3">
        <Input
          placeholder="Search branches…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="h-7 flex-shrink-0 text-xs"
        />

        {/* Flat branch list — protected first, then selectable */}
        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col">
            {[
              ...visible.filter((b) => b.isProtected),
              ...visible.filter((b) => !b.isProtected),
            ].map((b) => {
              if (b.isProtected) {
                return (
                  <div
                    key={b.name}
                    className="flex items-center gap-2 px-1 py-1.5 opacity-40"
                  >
                    <Lock className="size-3 shrink-0" />
                    <span className="min-w-0 flex-1 truncate font-mono text-xs">{b.name}</span>
                  </div>
                )
              }
              if (b.hasWorkspace) {
                return (
                  <div
                    key={b.name}
                    className="flex items-center gap-2 px-1 py-1.5 opacity-40"
                  >
                    <Check className="size-3 shrink-0 text-green-500" />
                    <span className="min-w-0 flex-1 truncate font-mono text-xs">{b.name}</span>
                  </div>
                )
              }
              return (
                <label
                  key={b.name}
                  className="flex cursor-pointer items-center gap-2 rounded px-1 py-1.5 text-xs hover:bg-accent/60"
                >
                  <Checkbox
                    checked={selected.has(b.name)}
                    onChange={() => toggleBranch(b.name)}
                    ariaLabel={b.name}
                  />
                  <span className="min-w-0 flex-1 truncate font-mono">{b.name}</span>
                </label>
              )
            })}
          </div>
        </ScrollArea>

        <Button
          size="sm"
          className="flex-shrink-0"
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
  )
}
