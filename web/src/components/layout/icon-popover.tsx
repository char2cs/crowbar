import { useRef, useState, type ReactNode } from 'react'
import { Upload, Smile, Star, Trash2, Pencil } from 'lucide-react'
import { Popover, PopoverTrigger, PopoverContent } from '@/components/ui/popover'
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { RepoAvatarImg } from './repo-avatar'
import { apiFetch } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'
import { openNativeDialog as openDialog } from '@/lib/native-dialog'
import { isTauri } from '@/lib/crowbar-bridge'

interface IconPopoverProps {
  /**
   * The entity's REST base — `/v0/projects/<id>` or
   * `/v0/projects/<id>/repos/<id>`. Every mutation below hangs off it, which is
   * the whole reason one component can serve both: the daemon exposes the same
   * four routes under each.
   */
  base: string
  /** Names the trigger and the preview image, for screen readers. */
  name: string
  /** The emoji currently set, if any. Wins over `iconUrl`. */
  emoji?: string
  /** The proxy URL of the stored image, if any. */
  iconUrl?: string
  /** Drawn at 20px in the row when neither an emoji nor an image is set. */
  fallback: ReactNode
  /** The same default, drawn large for the popover's preview. */
  fallbackLarge: ReactNode
  /**
   * Offer the GitHub owner-avatar button. Repos only: it reads the repo's
   * `origin` remote, and a project has none of its own.
   */
  github?: boolean
}

/**
 * The editable icon shared by the sidebar's two header rows.
 *
 * Clicking the mark — and only the mark, the click is stopped from bubbling to
 * the row — opens the editor; hovering it reveals a pencil saying it is
 * editable. It edits the ICON only: renaming stays on the name's double-click,
 * on both row types.
 *
 * Three states, in precedence order: an emoji, an uploaded image, or the
 * entity's own default. That order is the daemon's (see the icons package), not
 * this component's — setting an emoji clears a stored image server-side, so the
 * two can never both be live.
 */
export function IconPopover({
  base,
  name,
  emoji,
  iconUrl,
  fallback,
  fallbackLarge,
  github = false,
}: IconPopoverProps) {
  const [emojiInput, setEmojiInput] = useState('')
  const [showEmojiInput, setShowEmojiInput] = useState(false)
  const [loading, setLoading] = useState(false)
  // The icon proxy URL is stable, so the browser caches the image and will not
  // refetch after an upload/github/reset changes the bytes in place. Bump this
  // on every successful mutation and append it as a cache-busting query param so
  // both the popover preview and the row mark refresh.
  const [version, setVersion] = useState(0)
  const fileRef = useRef<HTMLInputElement>(null)

  const src = iconUrl ? `${iconUrl}${iconUrl.includes('?') ? '&' : '?'}v=${version}` : undefined

  /** Run a mutation, surface any failure, and refresh the cached image. */
  async function mutate(run: () => Promise<unknown>, failure: string) {
    setLoading(true)
    try {
      await run()
      setVersion((v) => v + 1)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : failure)
    } finally {
      setLoading(false)
    }
  }

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
    await mutate(
      () =>
        apiFetch(`${base}/icon`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path: selected }),
        }),
      'Failed to set icon',
    )
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    const form = new FormData()
    form.append('icon', file)
    await mutate(
      () => apiFetch(`${base}/icon`, { method: 'PUT', body: form }),
      'Failed to set icon',
    )
    if (fileRef.current) fileRef.current.value = ''
  }

  async function handleEmojiSubmit() {
    const value = emojiInput.trim()
    if (!value) return
    await mutate(
      () =>
        apiFetch(`${base}/icon/emoji`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ emoji: value }),
        }),
      'Failed to set emoji',
    )
    setEmojiInput('')
    setShowEmojiInput(false)
  }

  async function handleGithubAvatar() {
    setLoading(true)
    try {
      await apiFetch(`${base}/icon/github`, { method: 'PUT' })
      setVersion((v) => v + 1)
    } catch {
      toast.error(
        'Could not fetch the GitHub avatar',
        'Check that the repo has a GitHub origin remote and the gh CLI is installed and authenticated.',
      )
    } finally {
      setLoading(false)
    }
  }

  const handleReset = () =>
    mutate(() => apiFetch(`${base}/icon`, { method: 'DELETE' }), 'Failed to reset icon')

  // The 20px trigger mark, matching the row's own glyph sizing.
  const trigger = emoji ? (
    <span className="inline-flex h-5 w-5 items-center justify-center text-lg leading-none">
      {emoji}
    </span>
  ) : src ? (
    <RepoAvatarImg
      src={src}
      alt={name}
      className="h-5 w-5 rounded-md object-cover"
      fallback={fallback}
    />
  ) : (
    fallback
  )

  return (
    <Popover>
      <PopoverTrigger
        aria-label={`Edit ${name} icon`}
        className="group/entity-icon relative inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-md outline-none"
        onClick={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
      >
        {trigger}
        {/* Pencil overlay — appears only when hovering the mark itself. */}
        <span className="pointer-events-none absolute inset-0 hidden items-center justify-center rounded-md bg-black/45 group-hover/entity-icon:flex">
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
      >
        <div className="flex flex-col gap-3 p-3">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            Icon
          </p>

          <div className="flex justify-center">
            <Avatar className="size-14 rounded-xl text-base">
              {emoji ? (
                <AvatarFallback className="rounded-xl bg-transparent text-2xl">
                  {emoji}
                </AvatarFallback>
              ) : src ? (
                <AvatarImage src={src} alt={name} />
              ) : (
                <AvatarFallback className="rounded-xl bg-transparent">
                  {fallbackLarge}
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
              disabled={loading}
              onClick={() => {
                setShowEmojiInput(false)
                void handleUpload()
              }}
              className="flex-1 gap-1 text-muted-foreground hover:text-foreground"
            >
              <Upload className="size-3" />
              Upload
            </Button>
            <Button
              variant={showEmojiInput ? 'secondary' : 'ghost'}
              size="xs"
              disabled={loading}
              onClick={() => setShowEmojiInput((v) => !v)}
              className="flex-1 gap-1 text-muted-foreground hover:text-foreground"
            >
              <Smile className="size-3" />
              Emoji
            </Button>
            {github && (
              <Button
                variant="ghost"
                size="xs"
                disabled={loading}
                onClick={() => {
                  setShowEmojiInput(false)
                  void handleGithubAvatar()
                }}
                className="flex-1 gap-1 text-muted-foreground hover:text-foreground"
              >
                <Star className="size-3" />
                GitHub
              </Button>
            )}
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
                disabled={!emojiInput.trim() || loading}
                onClick={() => void handleEmojiSubmit()}
              >
                Set
              </Button>
            </div>
          )}

          {(emoji || iconUrl) && (
            <Button
              variant="ghost"
              size="xs"
              disabled={loading}
              onClick={() => void handleReset()}
              className="w-full gap-1 text-muted-foreground/60 hover:text-destructive"
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
