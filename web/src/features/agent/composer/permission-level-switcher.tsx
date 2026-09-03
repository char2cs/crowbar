import { useCallback, useRef, useState } from 'react'
import {
  PERMISSION_LEVEL_OPTIONS,
  setChatPermissionLevel,
  type PermissionLevel,
} from '@/features/agent/api/agent-api'
import { toast } from '@/features/window/stores/toast-store'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

// This control's own memory of its last CONFIRMED pick, per chat — not a
// fabricated read of backend truth, just this component's state surviving a
// remount. `ComposerChoice` is unkeyed in the composer's switch and
// `resolveComposerState` cycles it out of and back into 'choice' between
// distinct prompts in the same chat, which remounts this component (and would
// otherwise reset it to unset) moments after a real pick. `chatId` itself
// never changes on a live instance — the chat pane above it remounts wholesale
// on chat switch — so this needs no invalidation beyond the module's lifetime.
const lastPickedLevel = new Map<string, PermissionLevel>()

/**
 * A quick, write-only dial for THIS chat's own approval level, sitting beside
 * whatever answer controls the current prompt already offers.
 *
 * There is no GET for a single chat's level (only the global default has
 * one), so — same rule AgentModelPicker's "Provider default" row follows —
 * it never shows a value the backend cannot actually confirm. It starts from
 * whatever THIS control last confirmed for this chat (see `lastPickedLevel`),
 * or unset if it has never been touched.
 *
 * `availableLevels` is the SAME "absent capability, absent UI" rule
 * AgentModelPicker applies to models/efforts: a level the current provider
 * does not declare is not merely disabled here, it is not offered at all —
 * the server rejects an explicit pick of one just as firmly. Undefined or
 * empty (provider not yet resolved, or a provider declaring none) hides the
 * whole control rather than offering three options two of which would 400.
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
  // Fences overlapping writes the same way DefaultPermissionLevelSetting does:
  // a late settlement from an earlier pick must not roll back, or cache, over
  // a later one that already landed.
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
        .catch(() => {
          if (writeGeneration.current !== generation) return
          setLevel(previous)
          toast.error(
            'Could not set permission level',
            'Crowbar could not reach the daemon — try again.',
          )
        })
    },
    [level, wsId, chatId],
  )

  const options = PERMISSION_LEVEL_OPTIONS.filter((opt) => availableLevels?.includes(opt.value))
  // Absent capability, absent UI — the same rule AgentModelPicker follows for
  // a provider declaring no catalogue at all.
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
