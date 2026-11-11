import { useCallback, useEffect, useRef, useState } from 'react'
import {
  getDefaultPermissionLevel,
  PERMISSION_LEVEL_OPTIONS,
  updateDefaultPermissionLevel,
} from '@/features/agent/api/agent-providers-api'
import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import { toast } from '@/features/window/stores/toast-store'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SettingRow } from '../settings-section'
import { SETTINGS_CONTROL_WIDTHS } from '../settings-control-widths'

// How much of a new chat's tool approval Crowbar answers itself. Daemon-owned (applied when a chat
// starts), so it is GET/PUT /v0/settings/chat/permission-level, optimistic then reconciled.
export function DefaultPermissionLevelSetting() {
  const { level, change } = useDefaultPermissionLevel()
  return <PermissionLevelRow level={level} onChange={change} />
}

/** The local daemon's default permission level, or `hostId`'s, and the one way to change it. */
export function useDefaultPermissionLevel(hostId?: string) {
  const [level, setLevel] = useState<PermissionLevel>('guarded')

  useEffect(() => {
    let cancelled = false
    void (hostId === undefined ? getDefaultPermissionLevel() : getDefaultPermissionLevel(hostId))
      .then((resolved) => {
        if (!cancelled) setLevel(resolved)
      })
      .catch(() => {
        if (cancelled) return
        toast.error(
          'Could not load permission level',
          'Crowbar could not reach the daemon — try again.',
        )
      })
    return () => {
      cancelled = true
    }
  }, [hostId])

  // One writer can still fire twice before the first PUT settles; without this fence a late
  // rejection of the first pick would rewind the second, successful one.
  const writeGeneration = useRef(0)

  const change = useCallback(
    (value: PermissionLevel | null) => {
      if (!value) return
      const previous = level
      const generation = ++writeGeneration.current
      setLevel(value)
      const write =
        hostId === undefined
          ? updateDefaultPermissionLevel(value)
          : updateDefaultPermissionLevel(value, hostId)
      void write.catch(() => {
        if (writeGeneration.current !== generation) return // superseded by a later pick
        setLevel(previous)
        toast.error(
          'Could not save permission level',
          'Crowbar could not reach the daemon — try again.',
        )
      })
    },
    [level, hostId],
  )

  return { level, change }
}

export function PermissionLevelRow({
  level,
  onChange,
}: {
  level: PermissionLevel
  onChange: (value: PermissionLevel | null) => void
}) {
  return (
    <SettingRow
      label="Default permission level"
      description="How much of a new chat's tool-call approval is answered automatically."
    >
      <Select items={PERMISSION_LEVEL_OPTIONS} value={level} onValueChange={onChange}>
        <SelectTrigger
          data-testid="default-permission-level-trigger"
          size="sm"
          className={SETTINGS_CONTROL_WIDTHS.default}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {PERMISSION_LEVEL_OPTIONS.map((opt) => (
            <SelectItem
              key={opt.value}
              value={opt.value}
              data-testid={`default-permission-level-option-${opt.value}`}
            >
              {opt.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </SettingRow>
  )
}
