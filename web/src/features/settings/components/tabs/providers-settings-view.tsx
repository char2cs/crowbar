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
import type { AgentProvider } from '@/features/agent/api/agent-api'
import type { AgentProvidersStatus } from '@/features/settings/stores/agent-providers-store'
import Section from '../settings-section'
import { ProviderColumnHeader } from './provider-columns'
import { SortableProviderRow } from './sortable-provider-row'
import { reorderProviderIds } from './provider-preferences'

interface ProvidersSettingsViewProps {
  providers: AgentProvider[]
  status: AgentProvidersStatus
  onToggle: (id: string, enabled: boolean) => void
  onToggleTools: (id: string, enabled: boolean) => void
  /** The whole list in its new priority order. */
  onReorder: (orderedIds: string[]) => void
  /** The default permission level row. */
  permission: ReactNode
  /** Adding an agent from a descriptor file, where the surface offers it. */
  descriptor?: ReactNode
  footer?: ReactNode
}

/** The Agents tab as drawn, fed its list and handlers: the desktop dialog owns the list
 *  itself; the phone's settings page draws the main webview's snapshot of it. */
export function ProvidersSettingsView({
  providers,
  status,
  onToggle,
  onToggleTools,
  onReorder,
  permission,
  descriptor,
  footer,
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
    <Section title="Agents">
      {/* A Section's header is hidden when it is the first one in a tab
          (settings-section.tsx `first:[&>.settings-section-header]:hidden`),
          so this tab renders its own heading and intro in the BODY, mirroring
          that header's typography. Without them the group is a bare list of
          names that never says what an agent actually is. */}
      <div className="mb-2 px-1 py-1.5">
        <h3 className="font-heading text-[32px] text-foreground">Agents</h3>
        <p className="ui-font ui-text-sm text-muted-foreground">
          Agents are the coding CLIs Crowbar runs your chats on — Claude Code, Codex, and any others
          you add. Crowbar starts one for you when you open a chat, and each keeps its own session
          and login.
        </p>
      </div>
      {permission}
      {descriptor}
      {/* THREE STATES, NOT ONE SENTENCE. "No agents available." is a claim
          about the MACHINE, and it was being shown for two situations that
          assert nothing of the kind: a fetch still in flight, and a fetch that
          never got an answer. A user with both CLIs installed and enabled read
          it and believed Crowbar. */}
      {providers.length === 0 && status === 'loading' ? (
        <p
          data-testid="providers-loading"
          className="ui-font ui-text-sm px-1 py-2 text-muted-foreground"
        >
          Loading agents…
        </p>
      ) : providers.length === 0 && status === 'failed' ? (
        <p
          data-testid="providers-unavailable"
          className="ui-font ui-text-sm px-1 py-2 text-muted-foreground"
        >
          Crowbar could not load the agent list — it could not reach the daemon. Reopen this tab to
          try again.
        </p>
      ) : providers.length === 0 ? (
        <p className="ui-font ui-text-sm px-1 py-2 text-muted-foreground">No agents available.</p>
      ) : (
        <>
          {/* WHAT THE HEADER CANNOT SAY. The column titles now name each
              control, so this line no longer has to describe the dot. What a
              one-word header still cannot convey is the CONSEQUENCE of turning
              Tools off — that the agent keeps running and only loses its way
              into Crowbar — and that is what is left here. */}
          <p className="ui-font ui-text-sm px-1 pb-1 text-muted-foreground">
            Drag to set which agent a new chat opens first, and turn one off to hide it from every
            New chat surface. Tools lets that agent use Crowbar itself — reading your workspaces and
            leaving review comments; turn it off and the agent still runs, with no way in.
          </p>
          {/* Only over a real list: the loading, unavailable and empty states
              have no columns to title. */}
          <ProviderColumnHeader />
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleDragEnd}
          >
            <SortableContext items={providerIds} strategy={verticalListSortingStrategy}>
              <div className="space-y-1">
                {providers.map((provider) => (
                  <SortableProviderRow
                    key={provider.id}
                    provider={provider}
                    onToggle={onToggle}
                    onToggleTools={onToggleTools}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        </>
      )}
      {footer}
    </Section>
  )
}
