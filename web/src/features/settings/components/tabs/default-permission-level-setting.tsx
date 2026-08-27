import { useCallback, useEffect, useState } from 'react'
import {
  getDefaultPermissionLevel,
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

const PERMISSION_LEVEL_OPTIONS: ReadonlyArray<{ value: PermissionLevel; label: string }> = [
  { value: 'guarded', label: 'Guarded' },
  { value: 'trusted', label: 'Trusted' },
  { value: 'full-auto', label: 'Full Auto' },
]

/**
 * How much of a new chat's tool-call approval Crowbar answers automatically.
 * Unlike the rest of this tab's rows, this one is backend-owned rather than
 * the localStorage `updateSetting` path — the daemon applies it the moment a
 * new chat starts, so it talks straight to GET/PUT
 * /v0/settings/chat/permission-level, optimistic-set-then-reconcile like
 * `commit()` above.
 */
export function DefaultPermissionLevelSetting() {
  const [level, setLevel] = useState<PermissionLevel>('guarded')

  useEffect(() => {
    let cancelled = false
    void getDefaultPermissionLevel().then((resolved) => {
      if (!cancelled) setLevel(resolved)
    })
    return () => {
      cancelled = true
    }
  }, [])

  const handleChange = useCallback(
    (value: PermissionLevel | null) => {
      if (!value) return
      const next = value
      const previous = level
      setLevel(next)
      void updateDefaultPermissionLevel(next).catch(() => {
        setLevel(previous)
        toast.error(
          'Could not save permission level',
          'Crowbar could not reach the daemon — try again.',
        )
      })
    },
    [level],
  )

  return (
    <SettingRow
      label="Default permission level"
      description="How much of a new chat's tool-call approval is answered automatically."
    >
      <Select value={level} onValueChange={handleChange}>
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
