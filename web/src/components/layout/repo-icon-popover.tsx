import { useRef, useState } from 'react'
import { Upload, Smile, Star, Trash2, Pencil } from 'lucide-react'
import { Popover, PopoverTrigger, PopoverContent } from '@/components/ui/popover'
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { RepoAvatarImg } from './repo-avatar'
import { WorkspaceAgentSpinner } from './workspace-branch-icon'
import { apiFetch } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'
import { openNativeDialog as openDialog } from '@/lib/native-dialog'
import { isTauri } from '@/lib/crowbar-bridge'
import { cn } from '@/lib/utils'
import type { Repo } from '@/lib/store/sidebar'

/**
 * The repo avatar, turned into a CossUI Popover trigger for editing the repo's
 * icon. Clicking the avatar (and only the avatar — the click is stopped from
 * bubbling to the row) opens the popover; hovering it reveals a pencil overlay
 * signalling it is editable. While an agent turn is running (`defaultWorking`)
 * the avatar is replaced by the spinner and is not editable — the row keeps its
 * navigation click.
 *
 * Edits only the icon; repo rename stays on the name's double-click gesture.
 */
export function RepoIconPopover({ repo }: { repo: Repo }) {
  const repoBase = `/v0/projects/${repo.projectId ?? ''}/repos/${repo.id}`
  const [emojiInput, setEmojiInput] = useState('')
  const [showEmojiInput, setShowEmojiInput] = useState(false)
  const [iconLoading, setIconLoading] = useState(false)
  // The icon proxy URL (/repos/:r/icon) is stable, so the browser caches the
  // image and won't refetch after an upload/github/reset changes the bytes in
  // place. Bump this on every successful mutation and append it as a cache-
  // busting query param so both the popover preview and the row avatar refresh.
  const [iconVersion, setIconVersion] = useState(0)
  const fileRef = useRef<HTMLInputElement>(null)

  const isEmoji = repo.avatarURL?.startsWith('emoji:')
  const avatarSrc =
    !isEmoji && repo.avatarURL
      ? `${repo.avatarURL}${repo.avatarURL.includes('?') ? '&' : '?'}v=${iconVersion}`
      : undefined

  // Upload entry point. On the desktop the WKWebView crowbar:// transport cannot
  // carry a multipart/binary body, so we use the native file dialog to get an
  // absolute path and let the daemon read the file (PUT JSON {path}). In a real
  // browser there is no filesystem path, so fall back to the hidden <input>.
  async function handleUpload() {
    if (!isTauri()) {
      fileRef.current?.click()
      return
    }
    const selected = await openDialog({
      multiple: false,
      filters: [{ name: 'Images', extensions: ['png', 'jpg', 'jpeg', 'webp'] }],
    })
    if (typeof selected !== 'string') return
    setIconLoading(true)
    try {
      await apiFetch(`${repoBase}/icon`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: selected }),
      })
      setIconVersion((v) => v + 1)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to set icon')
    } finally {
      setIconLoading(false)
    }
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    setIconLoading(true)
    try {
      const form = new FormData()
      form.append('icon', file)
      await apiFetch(`${repoBase}/icon`, { method: 'PUT', body: form })
      setIconVersion((v) => v + 1)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to set icon')
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
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to set emoji')
    } finally {
      setIconLoading(false)
    }
  }

  async function handleGithubAvatar() {
    setIconLoading(true)
    try {
      await apiFetch(`${repoBase}/icon/github`, { method: 'PUT' })
      setIconVersion((v) => v + 1)
    } catch {
      toast.error(
        'Could not fetch the GitHub avatar',
        'Check that the repo has a GitHub origin remote and the gh CLI is installed and authenticated.',
      )
    } finally {
      setIconLoading(false)
    }
  }

  async function handleResetIcon() {
    setIconLoading(true)
    try {
      await apiFetch(`${repoBase}/icon`, { method: 'DELETE' })
      setIconVersion((v) => v + 1)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to reset icon')
    } finally {
      setIconLoading(false)
    }
  }

  // Small 5x5 trigger avatar mirroring the row's glyph sizing.
  const triggerAvatar = isEmoji ? (
    <span className="inline-flex h-5 w-5 items-center justify-center text-lg leading-none">
      {repo.avatarURL!.slice(6)}
    </span>
  ) : avatarSrc ? (
    <RepoAvatarImg
      src={avatarSrc}
      alt={repo.name}
      className="h-5 w-5 rounded-md object-cover"
      fallback={
        <span
          className={cn(
            'inline-flex h-5 w-5 items-center justify-center rounded-md px-1 text-[11px] font-bold text-primary-foreground',
            repo.avatarColor,
          )}
        >
          {repo.avatarLabel}
        </span>
      }
    />
  ) : (
    <span
      className={cn(
        'inline-flex h-5 w-5 items-center justify-center rounded-md px-1 text-[11px] font-bold text-primary-foreground',
        repo.avatarColor,
      )}
    >
      {repo.avatarLabel}
    </span>
  )

  // An agent turn is running: show the spinner in place of the avatar, no popover.
  // pointer-events-none lets the click fall through to the row (navigate).
  if (repo.defaultWorking) {
    return (
      <span
        className="pointer-events-none inline-flex h-5 w-5 shrink-0 items-center justify-center"
        data-oracle-id="repo-icon-popover-trigger"
      >
        <WorkspaceAgentSpinner />
      </span>
    )
  }

  return (
    <Popover>
      <PopoverTrigger
        aria-label={`Edit ${repo.name} icon`}
        className="group/repo-icon relative inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-md outline-none"
        onClick={(e) => e.stopPropagation()}
        data-oracle-id="repo-icon-popover-trigger"
      >
        {triggerAvatar}
        {/* Pencil overlay — appears only when hovering the avatar itself. */}
        <span className="pointer-events-none absolute inset-0 hidden items-center justify-center rounded-md bg-black/45 group-hover/repo-icon:flex">
          <Pencil className="size-2.5 text-white" />
        </span>
      </PopoverTrigger>
      <PopoverContent
        side="right"
        align="start"
        className="w-64 p-0"
        // The popover lives inside the row's clickable region; stop interactions
        // from bubbling out to the row's navigation handler.
        onClick={(e) => e.stopPropagation()}
        // Namespaced away from PopoverContent's own generic
        // `popover-popup` default — this is a call site of a shared
        // primitive, and surface.rs's registry requires a unique root
        // anchor per surface (the same reason detach-holder-modal-popup
        // and repo-import-dialog-popup are namespaced, P3.51).
        data-oracle-id="repo-icon-popover-popup"
      >
        <div className="flex flex-col gap-3 p-3">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            Icon
          </p>

          <div className="flex justify-center">
            <Avatar className="size-14 rounded-xl text-base">
              {isEmoji ? (
                <AvatarFallback className="rounded-xl bg-transparent text-2xl">
                  {repo.avatarURL!.slice(6)}
                </AvatarFallback>
              ) : avatarSrc ? (
                <AvatarImage src={avatarSrc} alt={repo.name} />
              ) : (
                <AvatarFallback
                  className={cn(
                    'rounded-xl text-sm font-bold text-primary-foreground',
                    repo.avatarColor,
                  )}
                >
                  {repo.avatarLabel}
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
              onClick={() => {
                setShowEmojiInput(false)
                void handleUpload()
              }}
              className="flex-1 gap-1 text-muted-foreground hover:text-foreground"
              data-oracle-content-sized="true"
              data-oracle-id="repo-icon-popover-upload"
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
              data-oracle-content-sized="true"
              data-oracle-id="repo-icon-popover-emoji"
            >
              <Smile className="size-3" />
              Emoji
            </Button>
            <Button
              variant="ghost"
              size="xs"
              disabled={iconLoading}
              onClick={() => {
                setShowEmojiInput(false)
                void handleGithubAvatar()
              }}
              className="flex-1 gap-1 text-muted-foreground hover:text-foreground"
              data-oracle-content-sized="true"
              data-oracle-id="repo-icon-popover-github"
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
                onKeyDown={(e) => {
                  if (e.key === 'Enter') void handleEmojiSubmit()
                }}
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
                data-oracle-id="repo-icon-popover-emoji-submit"
              >
                Set
              </Button>
            </div>
          )}

          {repo.avatarURL && (
            <Button
              variant="ghost"
              size="xs"
              disabled={iconLoading}
              onClick={() => void handleResetIcon()}
              className="w-full gap-1 text-muted-foreground/60 hover:text-destructive"
              data-oracle-id="repo-icon-popover-reset"
            >
              <Trash2 className="size-3" />
              Reset to default
            </Button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
