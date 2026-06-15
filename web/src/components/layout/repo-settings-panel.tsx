import { useState, useEffect, useRef } from 'react'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'
import { apiFetch } from '@/lib/api'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useSidebarStore } from '@/lib/store/sidebar'
import { Lock, Check, Trash2, Upload, Star, Smile } from 'lucide-react'
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
  const [showEmojiInput, setShowEmojiInput] = useState(false)
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
      setShowEmojiInput(false)
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
            variant="outline"
            size="sm"
            disabled={iconLoading}
            onClick={() => { setShowEmojiInput(false); fileRef.current?.click() }}
            className="h-7 flex-1 gap-1.5 text-[11px]"
          >
            <Upload className="size-3" />
            Upload
          </Button>
          <Button
            variant={showEmojiInput ? 'secondary' : 'outline'}
            size="sm"
            disabled={iconLoading}
            onClick={() => setShowEmojiInput((v) => !v)}
            className="h-7 flex-1 gap-1.5 text-[11px]"
          >
            <Smile className="size-3" />
            Emoji
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={iconLoading}
            onClick={() => { setShowEmojiInput(false); void handleGithubAvatar() }}
            className="h-7 flex-1 gap-1.5 text-[11px]"
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
            size="sm"
            disabled={iconLoading}
            onClick={() => void handleResetIcon()}
            className="h-7 text-[11px] text-muted-foreground hover:text-destructive"
          >
            <Trash2 className="mr-1.5 size-3" />
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
