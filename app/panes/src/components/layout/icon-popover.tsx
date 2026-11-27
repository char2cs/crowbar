import { useRef, useState, type ReactNode } from 'react'
import {
  PencilSimpleIcon,
  SmileyIcon,
  StarIcon,
  TrashIcon,
  UploadSimpleIcon,
} from '@phosphor-icons/react'
import { Popover, PopoverTrigger, PopoverContent } from '@/components/ui/popover'
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { RepoAvatarImg } from './repo-avatar'
import { apiFetch } from '@/lib/api/fetch'
import { toast } from '@/features/window/stores/toast-store'
import { putIconFile } from './icon-upload'
import { openNativeDialog as openDialog } from '@/lib/native-dialog'
import { isTauri } from '@/lib/crowbar-bridge'
import { report } from '@/lib/errors/report'

interface IconPopoverProps {
  base: string
  name: string
  emoji?: string
  iconUrl?: string
  fallback: ReactNode
  fallbackLarge: ReactNode
  trigger?: (version: number) => ReactNode
  github?: boolean
}

export function IconPopover({
  base,
  name,
  emoji,
  iconUrl,
  fallback,
  fallbackLarge,
  trigger: triggerOverride,
  github = false,
}: IconPopoverProps) {
  const [emojiInput, setEmojiInput] = useState('')
  const [showEmojiInput, setShowEmojiInput] = useState(false)
  const [loading, setLoading] = useState(false)
  const [version, setVersion] = useState(0)
  const fileRef = useRef<HTMLInputElement>(null)

  const src = iconUrl ? `${iconUrl}${iconUrl.includes('?') ? '&' : '?'}v=${version}` : undefined

  async function mutate(run: () => Promise<unknown>, failure: string) {
    setLoading(true)
    try {
      await run()
      setVersion((v) => v + 1)
    } catch (err) {
      report(err, { action: failure })
    } finally {
      setLoading(false)
    }
  }

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
    await mutate(() => putIconFile(`${base}/icon`, selected), 'Failed to set icon')
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

  const handleReset = () => {
    return mutate(() => apiFetch(`${base}/icon`, { method: 'DELETE' }), 'Failed to reset icon')
  }

  const trigger = triggerOverride ? (
    triggerOverride(version)
  ) : emoji ? (
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
        className="group/entity-icon relative inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-md align-middle outline-none focus-visible:ring-2 focus-visible:ring-ring"
        onClick={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
      >
        {trigger}
        <span className="pointer-events-none absolute inset-0 hidden items-center justify-center rounded-md bg-black/45 group-hover/entity-icon:flex">
          <PencilSimpleIcon className="size-2.5 text-white" />
        </span>
      </PopoverTrigger>
      <PopoverContent
        side="right"
        align="start"
        className="w-64 p-0"
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
              ) : (
                <>
                  {src && <AvatarImage src={src} alt={name} />}
                  <AvatarFallback className="rounded-xl bg-transparent">
                    {fallbackLarge}
                  </AvatarFallback>
                </>
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
              <UploadSimpleIcon className="size-3" />
              Upload
            </Button>
            <Button
              variant={showEmojiInput ? 'secondary' : 'ghost'}
              size="xs"
              disabled={loading}
              onClick={() => setShowEmojiInput((v) => !v)}
              className="flex-1 gap-1 text-muted-foreground hover:text-foreground"
            >
              <SmileyIcon className="size-3" />
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
                <StarIcon className="size-3" />
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
              <TrashIcon className="size-3" />
              Reset to default
            </Button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
