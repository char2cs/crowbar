import { useCallback, useEffect, useRef, useState } from 'react'
import { Shield } from '@phosphor-icons/react'
import {
  getDefaultPermissionLevel,
  PERMISSION_LEVEL_OPTIONS,
  type PermissionLevel,
} from '@/features/agent/api/agent-providers-api'
import { setChatPermissionLevel } from '@/features/agent/api/agent-chat-commands'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { CheckIcon, UpDownIcon } from '@/features/agent/shared/agent-icons'
import { toast } from '@/features/window/stores/toast-store'
import { toastHostFailure } from '@/features/hosts/lib/host-failure-notice'
import { useChatHost } from '@/lib/host-resolve'
import { Dropdown } from '@/components/ui/dropdown'

/** The default level of the chat's host while the chat pins none; null until it is known. */
function useInheritedLevel(chatId: string, inherits: boolean): PermissionLevel | null {
  const hostId = useChatHost(chatId)
  const [known, setKnown] = useState<{ hostId: string; level: PermissionLevel } | null>(null)
  useEffect(() => {
    if (!inherits || hostId === null) return
    let live = true
    getDefaultPermissionLevel(hostId).then(
      (level) => live && setKnown({ hostId, level }),
      () => live && setKnown(null),
    )
    return () => {
      live = false
    }
  }, [hostId, inherits])
  return known?.hostId === hostId ? known.level : null
}

/** The level a spawn would run at: `wanted` if offered, else the nearest more
 *  permissive offered one, as the daemon resolves it. */
function offeredLevel(wanted: PermissionLevel, offered: PermissionLevel[]): PermissionLevel {
  const order = PERMISSION_LEVEL_OPTIONS.map((opt) => opt.value)
  const offers = new Set(offered)
  return order.slice(order.indexOf(wanted)).find((l) => offers.has(l)) ?? offered.at(-1) ?? wanted
}

export interface PermissionTarget {
  wsId: string
  chatId: string
  /** The levels the provider declares; absent or empty offers none. */
  levels?: PermissionLevel[]
}

/** The level the chat runs at and the write that changes it, for every surface offering the
 *  choice. A null `target` is inert, so a closed picker fetches nothing. */
export function usePermissionLevel(target: PermissionTarget | null) {
  const wsId = target?.wsId ?? ''
  const chatId = target?.chatId ?? ''
  const pinned = useChatField<PermissionLevel | ''>(chatId, (c) => c?.permissionLevel ?? '')
  // The person's pick shows from the moment it is made; a failed one falls back.
  const [picked, setPicked] = useState<PermissionLevel | null>(null)
  const inherited = useInheritedLevel(chatId, target !== null && !picked && !pinned)
  // A late settlement from an earlier pick must not roll back over a later one.
  const writeGeneration = useRef(0)

  const setLevel = useCallback(
    (value: PermissionLevel | '' | null) => {
      if (!value) return
      const generation = ++writeGeneration.current
      setPicked(value)
      setChatPermissionLevel(wsId, chatId, value).catch((err: unknown) => {
        if (writeGeneration.current !== generation) return // superseded by a later pick
        setPicked(null)
        if (toastHostFailure(err)) return
        toast.error(
          'Could not set permission level',
          'Crowbar could not reach the daemon — try again.',
        )
      })
    },
    [wsId, chatId],
  )

  const options = PERMISSION_LEVEL_OPTIONS.filter((opt) => target?.levels?.includes(opt.value))
  const offered = options.map((opt) => opt.value)
  const level: PermissionLevel | '' =
    picked ?? (pinned || (inherited && offered.length > 0 ? offeredLevel(inherited, offered) : ''))
  return { options, level, setLevel }
}

/** The footer's permission chip: the current level, opening the choice as a menu. */
export function PermissionLevelChip({ target }: { target: PermissionTarget }) {
  const { options, level, setLevel } = usePermissionLevel(target)
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  const close = useCallback(() => setOpen(false), [])
  if (options.length === 0) return null
  const current = options.find((opt) => opt.value === level)

  return (
    <>
      <button
        ref={anchorRef}
        type="button"
        className="chip"
        data-testid="chat-permission-level-chip"
        title="What the agent may do without asking"
        aria-label={`Permission level for this chat: ${current?.label ?? 'not set'}`}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((was) => !was)}
      >
        <Shield size={14} />
        <span>{current?.label ?? 'Permission'}</span>
        <UpDownIcon size={12} className="opacity-55" />
      </button>
      <Dropdown
        isOpen={open}
        onClose={close}
        anchorRef={anchorRef}
        anchorSide="top"
        anchorAlign="start"
        style={{ width: 332 }}
      >
        <div role="menu" aria-label="Permission level" className="p-1">
          {options.map((opt) => (
            <button
              key={opt.value}
              type="button"
              role="menuitemradio"
              aria-checked={opt.value === level}
              className="lvl-row"
              data-selected={opt.value === level || undefined}
              data-risk={opt.value === 'full-auto' || undefined}
              onClick={() => {
                setLevel(opt.value)
                close()
              }}
            >
              <span>
                <b>{opt.label}</b>
                <small>{opt.description}</small>
              </span>
              {opt.value === level && <CheckIcon size={12} className="lvl-check" />}
            </button>
          ))}
        </div>
      </Dropdown>
    </>
  )
}
