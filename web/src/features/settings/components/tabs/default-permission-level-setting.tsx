import { useCallback, useEffect, useRef, useState } from 'react'
import {
  getDefaultPermissionLevel,
  PERMISSION_LEVEL_OPTIONS,
  updateDefaultPermissionLevel,
} from '@/features/agent/api/agent-api'
import type { PermissionLevel } from '@/features/agent/api/agent-api'
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

/**
 * How much of a new chat's tool-call approval Crowbar answers automatically.
 * Unlike the rest of this tab's rows, this one is backend-owned rather than
 * the localStorage `updateSetting` path — the daemon applies it the moment a
 * new chat starts, so it talks straight to GET/PUT
 * /v0/settings/chat/permission-level, optimistic-set-then-reconcile like
 * `ProvidersSettings`'s own `commit()`.
 */
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

  // Scoped to THIS component's own sequence of writes — not
  // agent-providers-store's generation counter, which arbitrates several call
  // sites sharing one array. Here there is one writer, but that writer can
  // still fire twice before the first PUT settles (pick Trusted, then Full
  // Auto before Trusted's response lands): without a fence, a late rejection
  // for the first pick would rewind the second, already-successful one.
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
