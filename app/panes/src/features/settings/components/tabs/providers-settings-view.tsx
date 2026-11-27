import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import type { DragEndEvent } from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { useCallback, useMemo, type ReactNode } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { Button } from '@/components/ui/button'
import type { AgentProvidersStatus } from '@/features/settings/stores/agent-providers-store'
import { cn } from '@/utils/cn'
import { IS_MOBILE } from '@/utils/platform'
import Section, { SettingsCard } from '../settings-section'

const NOTE_INSET = IS_MOBILE ? 'px-4' : 'px-1'
import { ProviderColumnHeader } from './provider-columns'
import { SortableProviderRow } from './sortable-provider-row'
import { reorderProviderIds } from './provider-preferences'

interface ProvidersSettingsViewProps {
  providers: AgentProvider[]
  status: AgentProvidersStatus
  onToggle: (id: string, enabled: boolean) => void
  onToggleTools: (id: string, enabled: boolean) => void
  onReorder: (orderedIds: string[]) => void
  permission: ReactNode
  descriptor?: ReactNode
  footer?: ReactNode
  onRetry?: () => void
}

export function ProvidersSettingsView({
  providers,
  status,
  onToggle,
  onToggleTools,
  onReorder,
  permission,
  descriptor,
  footer,
  onRetry,
}: ProvidersSettingsViewProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      if (!event.over) return
      const activeId = String(event.active.id)
      const overId = String(event.over.id)
      if (activeId === overId) return
      const orderedIds = reorderProviderIds(
        providers.map((p) => p.id),
        activeId,
        overId,
      )
      onReorder(orderedIds)
    },
    [providers, onReorder],
  )

  const providerIds = useMemo(() => providers.map((p) => p.id), [providers])

  return (
    <Section title="Agents" plain>
      <div className={cn('mb-2', IS_MOBILE ? 'px-4' : 'px-1 py-1.5')}>
        {!IS_MOBILE && <h3 className="font-heading text-[32px] text-foreground">Agents</h3>}
        <p className="ui-font ui-text-sm text-muted-foreground">
          Agents are the coding CLIs Crowbar runs your chats on — Claude Code, Codex, and any others
          you add. Crowbar starts one for you when you open a chat, and each keeps its own session
          and login.
        </p>
      </div>
      {permission && <SettingsCard>{permission}</SettingsCard>}
      {descriptor}
      {providers.length === 0 && status === 'loading' ? (
        <p
          data-testid="providers-loading"
          className={cn('ui-font ui-text-sm py-2 text-muted-foreground', NOTE_INSET)}
        >
          Loading agents…
        </p>
      ) : providers.length === 0 && status === 'failed' ? (
        <div
          data-testid="providers-unavailable"
          className={cn(
            'ui-font ui-text-sm flex items-center gap-2 py-2 text-muted-foreground',
            NOTE_INSET,
          )}
        >
          Crowbar could not load the agent list — it could not reach the daemon.
          {onRetry && (
            <Button size="xs" variant="outline" onClick={onRetry}>
              Retry
            </Button>
          )}
        </div>
      ) : providers.length === 0 ? (
        <p className={cn('ui-font ui-text-sm py-2 text-muted-foreground', NOTE_INSET)}>
          No agents available.
        </p>
      ) : (
        <>
          <p className={cn('ui-font ui-text-sm pb-1 text-muted-foreground', NOTE_INSET)}>
            Drag to set which agent a new chat opens first, and turn one off to hide it from every
            New chat surface. Tools lets that agent use Crowbar itself — reading your workspaces and
            leaving review comments; turn it off and the agent still runs, with no way in.
          </p>
          <ProviderColumnHeader />
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleDragEnd}
          >
            <SortableContext items={providerIds} strategy={verticalListSortingStrategy}>
              <SettingsCard className={IS_MOBILE ? undefined : 'space-y-1'}>
                {providers.map((provider) => (
                  <SortableProviderRow
                    key={provider.id}
                    provider={provider}
                    onToggle={onToggle}
                    onToggleTools={onToggleTools}
                  />
                ))}
              </SettingsCard>
            </SortableContext>
          </DndContext>
        </>
      )}
      {footer}
    </Section>
  )
}
