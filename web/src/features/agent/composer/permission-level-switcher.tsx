import { useCallback, useRef, useState } from 'react'
import { setChatPermissionLevel, type PermissionLevel } from '@/features/agent/api/agent-api'
import { toast } from '@/features/window/stores/toast-store'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const PERMISSION_LEVEL_OPTIONS: ReadonlyArray<{ value: PermissionLevel; label: string }> = [
  { value: 'guarded', label: 'Guarded' },
  { value: 'trusted', label: 'Trusted' },
  { value: 'full-auto', label: 'Full Auto' },
]

/**
 * A quick, write-only dial for THIS chat's own approval level, sitting beside
 * whatever answer controls the current prompt already offers.
 *
 * There is no GET for a single chat's level (only the global default has
 * one), so — same rule AgentModelPicker's "Provider default" row follows —
 * it never shows a value the backend cannot actually confirm. It starts
 * unset and only ever shows what THIS control has itself picked.
 */
export function PermissionLevelSwitcher({ wsId, chatId }: { wsId: string; chatId: string }) {
  const [level, setLevel] = useState<PermissionLevel | ''>('')
  // Fences overlapping writes the same way DefaultPermissionLevelSetting does:
  // a late rejection from an earlier pick must not roll back a later one that
  // already succeeded.
  const writeGeneration = useRef(0)

  const handleChange = useCallback(
    (value: PermissionLevel | '' | null) => {
      if (!value) return
      const previous = level
      const generation = ++writeGeneration.current
      setLevel(value)
      void setChatPermissionLevel(wsId, chatId, value).catch(() => {
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
        {PERMISSION_LEVEL_OPTIONS.map((opt) => (
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
