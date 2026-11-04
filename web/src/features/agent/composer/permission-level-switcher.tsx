import { useCallback, useRef, useState } from 'react'
import {
  PERMISSION_LEVEL_OPTIONS,
  type PermissionLevel,
} from '@/features/agent/api/agent-providers-api'
import { setChatPermissionLevel } from '@/features/agent/api/agent-chat-commands'
import { toast } from '@/features/window/stores/toast-store'
import { toastHostFailure } from '@/features/hosts/lib/host-failure-notice'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

// The last confirmed pick per chat, so it survives the remount between prompts.
// Not backend truth: there is no GET for a chat's level.
const lastPickedLevel = new Map<string, PermissionLevel>()

/**
 * Write-only: it never shows a level the backend cannot confirm, and offers only
 * levels the provider declares — the server rejects the rest.
 */
export function PermissionLevelSwitcher({
  wsId,
  chatId,
  availableLevels,
}: {
  wsId: string
  chatId: string
  availableLevels?: PermissionLevel[]
}) {
  const [level, setLevel] = useState<PermissionLevel | ''>(() => lastPickedLevel.get(chatId) ?? '')
  // A late settlement from an earlier pick must not roll back over a later one.
  const writeGeneration = useRef(0)

  const handleChange = useCallback(
    (value: PermissionLevel | '' | null) => {
      if (!value) return
      const previous = level
      const generation = ++writeGeneration.current
      setLevel(value)
      setChatPermissionLevel(wsId, chatId, value)
        .then(() => {
          if (writeGeneration.current !== generation) return // superseded by a later pick
          lastPickedLevel.set(chatId, value)
        })
        .catch((err: unknown) => {
          if (writeGeneration.current !== generation) return
          setLevel(previous)
          if (toastHostFailure(err)) return
          toast.error(
            'Could not set permission level',
            'Crowbar could not reach the daemon — try again.',
          )
        })
    },
    [level, wsId, chatId],
  )

  const options = PERMISSION_LEVEL_OPTIONS.filter((opt) => availableLevels?.includes(opt.value))
  if (options.length === 0) return null

  return (
    <Select value={level} onValueChange={handleChange}>
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
