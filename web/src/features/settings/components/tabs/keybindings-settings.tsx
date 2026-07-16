import { Keyboard, Warning } from '@phosphor-icons/react'
import { useCallback, useEffect, useEffectEvent, useMemo, useState } from 'react'
import { KEYMAP_PRESET_OPTIONS } from '@/features/keymaps/defaults/keybinding-presets'
import { useKeymapStore } from '@/features/keymaps/stores/store'
import { COMMANDS } from '@/features/keymaps/registry'
import { chordFromEvent, formatChord } from '@/features/keymaps/utils/chord'
import {
  findConflictingCommands,
  getEffectiveBindings,
} from '@/features/keymaps/utils/effective-keymaps'
import type {
  Command,
  CommandCategory,
  EffectiveBinding,
  KeymapPresetId,
} from '@/features/keymaps/types'
import { getCommand } from '@/features/keymaps/registry'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/utils/cn'
import Section from '../settings-section'
import { SETTINGS_CONTROL_WIDTHS } from '../settings-control-widths'

const CATEGORY_ORDER: CommandCategory[] = ['Navigation', 'Panes', 'Tabs', 'Editor']

/** Inline key-capture button for rebinding a single command. */
function RebindControl({
  command,
  binding,
  conflictCommandIds,
  onCapture,
}: {
  command: Command
  binding: EffectiveBinding
  conflictCommandIds: string[]
  onCapture: (chord: string) => void
}) {
  const [capturing, setCapturing] = useState(false)

  // Read the latest onCapture via an Effect Event so the capture listener
  // subscribes once per capturing session instead of re-subscribing when the
  // parent hands down a new onCapture identity.
  const onChordCaptured = useEffectEvent((chord: string) => {
    onCapture(chord)
    setCapturing(false)
  })
  useEffect(() => {
    if (!capturing) return
    const handler = (e: KeyboardEvent) => {
      e.preventDefault()
      e.stopPropagation()
      if (e.key === 'Escape') {
        setCapturing(false)
        return
      }
      const chord = chordFromEvent(e)
      if (!chord) return // bare modifier — keep waiting
      onChordCaptured(chord)
    }
    window.addEventListener('keydown', handler, { capture: true })
    return () => window.removeEventListener('keydown', handler, { capture: true })
  }, [capturing])

  const hasConflict = conflictCommandIds.length > 0
  const conflictLabels = conflictCommandIds.map((id) => getCommand(id)?.label ?? id).join(', ')

  return (
    <div className="flex items-center gap-2">
      {hasConflict && (
        <span
          className="text-destructive flex items-center gap-1"
          title={`Also bound to: ${conflictLabels}`}
        >
          <Warning size={14} weight="duotone" />
        </span>
      )}
      <Button
        type="button"
        variant={capturing ? 'default' : 'outline'}
        compact
        disabled={!command.liveEditable}
        onClick={() => setCapturing((c) => !c)}
        className={cn(
          'ui-text-sm min-w-[88px] justify-center font-mono',
          hasConflict && 'border-destructive text-destructive',
        )}
      >
        {capturing ? 'Press keys…' : formatChord(binding.chord) || 'Unbound'}
      </Button>
    </div>
  )
}

export const KeybindingsSettings = () => {
  const preset = useKeymapStore.use.preset()
  const overrides = useKeymapStore.use.overrides()
  const actions = useKeymapStore.use.actions()

  const bindings = useMemo(() => getEffectiveBindings({ preset, overrides }), [preset, overrides])
  const bindingByCommand = useMemo(() => {
    const map = new Map<string, EffectiveBinding>()
    for (const b of bindings) map.set(b.commandId, b)
    return map
  }, [bindings])

  const grouped = useMemo(() => {
    const map = new Map<CommandCategory, Command[]>()
    for (const command of COMMANDS) {
      const list = map.get(command.category) ?? []
      list.push(command)
      map.set(command.category, list)
    }
    return map
  }, [])

  const handleCapture = useCallback(
    (commandId: string, chord: string) => {
      actions.setOverride(commandId, chord)
    },
    [actions],
  )

  const conflictsFor = useCallback(
    (commandId: string): string[] => {
      const binding = bindingByCommand.get(commandId)
      if (!binding) return []
      return findConflictingCommands(commandId, binding.chord, { preset, overrides })
    },
    [bindingByCommand, preset, overrides],
  )

  const hasOverrides = Object.keys(overrides).length > 0

  return (
    <div className="space-y-4">
      <Section
        title="Preset"
        description="Choose a base keybinding set. Switching presets re-resolves every command's live shortcut. Per-command overrides below take precedence over the preset."
      >
        <div className="flex items-center justify-between px-1 py-1">
          <div className="ui-font ui-text-sm text-muted-foreground">Active preset</div>
          <Select
            value={preset}
            onValueChange={(value) => {
              if (value) actions.setPreset(value as KeymapPresetId)
            }}
          >
            <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS.xwide}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {KEYMAP_PRESET_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <p className="ui-font ui-text-sm px-1 text-muted-foreground">
          Note: the standard IDE presets (VS Code, JetBrains, Sublime, Xcode, Atom, Emacs, Zed) are
          not yet provided — only Default and one alternate (Compact) ship real bindings.
        </p>
      </Section>

      {CATEGORY_ORDER.map((category) => {
        const commands = grouped.get(category)
        if (!commands || commands.length === 0) return null
        return (
          <Section key={category} title={category}>
            <div className="divide-y divide-border/60">
              {commands.map((command) => {
                const binding = bindingByCommand.get(command.id)
                if (!binding) return null
                return (
                  <div
                    key={command.id}
                    className="flex items-center justify-between gap-3 px-1 py-2"
                  >
                    <div className="min-w-0">
                      <div className="ui-font ui-text-sm flex items-center gap-2 text-foreground">
                        <Keyboard size={14} className="shrink-0 text-muted-foreground" />
                        <span className="truncate">{command.label}</span>
                        {!command.liveEditable && (
                          <span className="ui-text-xs rounded bg-muted px-1.5 py-0.5 text-muted-foreground">
                            display-only
                          </span>
                        )}
                        {binding.source !== 'default' && (
                          <span className="ui-text-xs text-accent-foreground">
                            {binding.source === 'user' ? 'custom' : 'preset'}
                          </span>
                        )}
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <RebindControl
                        command={command}
                        binding={binding}
                        conflictCommandIds={conflictsFor(command.id)}
                        onCapture={(chord) => handleCapture(command.id, chord)}
                      />
                      {command.id in overrides && (
                        <Button
                          type="button"
                          variant="ghost"
                          compact
                          className="ui-text-xs text-muted-foreground"
                          onClick={() => actions.clearOverride(command.id)}
                        >
                          Reset
                        </Button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </Section>
        )
      })}

      {hasOverrides && (
        <div className="px-1">
          <Button type="button" variant="outline" compact onClick={() => actions.resetOverrides()}>
            Reset all custom keybindings
          </Button>
        </div>
      )}
    </div>
  )
}
