import { useCallback, useEffect, useRef, useState } from 'react'
import {
  getDefaultPermissionLevel,
  PERMISSION_LEVEL_OPTIONS,
  type PermissionLevel,
} from '@/features/agent/api/agent-providers-api'
import { setChatPermissionLevel } from '@/features/agent/api/agent-chat-commands'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { toast } from '@/features/window/stores/toast-store'
import { toastHostFailure } from '@/features/hosts/lib/host-failure-notice'
import { useChatHost } from '@/lib/host-resolve'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

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
  const from = order.indexOf(wanted)
  return order.slice(from).find((l) => offered.includes(l)) ?? offered.at(-1) ?? wanted
}

/** Shows the level the chat is pinned to (or inherits), and offers only levels the provider
 *  declares — the server rejects the rest. */
export function PermissionLevelSwitcher({
  wsId,
  chatId,
  availableLevels,
}: {
  wsId: string
  chatId: string
  availableLevels?: PermissionLevel[]
}) {
  const pinned = useChatField<PermissionLevel | ''>(chatId, (c) => c?.permissionLevel ?? '')
  // The person's pick shows from the moment it is made; a failed one falls back.
  const [picked, setPicked] = useState<PermissionLevel | null>(null)
  const inherited = useInheritedLevel(chatId, !picked && !pinned)
  // A late settlement from an earlier pick must not roll back over a later one.
  const writeGeneration = useRef(0)

  const handleChange = useCallback(
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

  const options = PERMISSION_LEVEL_OPTIONS.filter((opt) => availableLevels?.includes(opt.value))
  if (options.length === 0) return null
  const offered = options.map((opt) => opt.value)
  const level: PermissionLevel | '' =
    picked ?? (pinned || (inherited ? offeredLevel(inherited, offered) : ''))

  return (
    <Select items={options} value={level} onValueChange={handleChange}>
      <SelectTrigger
        data-testid="chat-permission-level-trigger"
        size="sm"
        aria-label="Permission level for this chat"
        className="w-32 max-w-full"
      >
        <SelectValue placeholder="Permission level" />
      </SelectTrigger>
      <SelectContent>
        {options.map((opt) => (
          <SelectItem
            key={opt.value}
            value={opt.value}
            data-testid={`chat-permission-level-option-${opt.value}`}
          >
            {opt.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
