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
import { useCallback, useEffect, useMemo } from 'react'
import { updateProviderPreferences } from '@/features/agent/api/agent-api'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import {
  beginProviderWrite,
  isLatestProviderWrite,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { toast } from '@/features/window/stores/toast-store'
import Section from '../settings-section'
import { ChatPresentationSetting } from './chat-presentation-setting'
import { DefaultPermissionLevelSetting } from './default-permission-level-setting'
import { ProviderColumnHeader } from './provider-columns'
import { SortableProviderRow } from './sortable-provider-row'
import {
  applyProviderPreferences,
  buildProviderPreferences,
  providerDisabledMap,
  reorderProviderIds,
} from './provider-preferences'
import type { ProviderFlags } from './provider-preferences'

/**
 * The Agents settings tab: the enriched, priority-ordered provider list, drag to
 * reorder priority, a switch to disable one.
 *
 * "Agents" is the USER-FACING name only (spec §11). `usecases/agent`,
 * `agentrunner` and `agentchat` already overload that word inside Crowbar, and
 * Claude has its own `--agent`, so nothing below the copy moves: the tab id, the
 * component, the store, the DTOs and the wire all still say provider.
 *
 * IT READS THE GLOBAL PROVIDER STORE, NOT THE ACTIVE WORKSPACE. Providers are
 * machine-level — the daemon says as much (ListProviders: "workspaceID is only
 * used to resolve crowbar home") — but the frontend's only copy used to live in
 * the per-workspace store, and this dialog opens from anywhere. With no workspace
 * in view that copy is empty, and the tab announced "No providers available." to
 * a user with both CLIs installed and enabled. It also fetches on mount, so a
 * workspace whose own seed was lost (see use-workspace-agent-chats-stream) is
 * repaired by opening this tab rather than staying dead.
 *
 * This is the first backend-persisted settings tab: on any change it PUTs the
 * COMPLETE ordered preference set and reconciles from the server's freshly
 * resolved response (server truth) — deliberately NOT the localStorage
 * `updateSetting` path other tabs use, because provider state is server-owned.
 */
export const ProvidersSettings = () => {
  const providers = useAgentProvidersStore((s) => s.providers)
  const status = useAgentProvidersStore((s) => s.status)

  // Refresh on open. The daemon exposes providers only under a workspace/home
  // scope even though the data is global, so with no scope at all we can only
  // show what is already known — and say so plainly when that is nothing.
  useEffect(() => {
    const wsId = getActiveWorkspaceId()
    if (!wsId) {
      useAgentProvidersStore.getState().markUnavailable()
      return
    }
    void useAgentProvidersStore
      .getState()
      .load(wsId)
      .then((resolved) => {
        // Repair the workspace copy too — it is what the chat surfaces read, and
        // a lost seed there is exactly what sends the user to this tab.
        if (resolved) getActiveWorkspaceStoreRef()?.getState().setAgentProviders(resolved)
      })
  }, [])

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  // BOTH copies, always together. The global one is what this tab renders; the
  // per-workspace one is what the chat surfaces render. Letting a write reach
  // only one of them is how they drift.
  const publish = useCallback((resolved: AgentProvider[]) => {
    useAgentProvidersStore.getState().setProviders(resolved)
    getActiveWorkspaceStoreRef()?.getState().setAgentProviders(resolved)
  }, [])

  // THE ONLY WRITE PATH. Every change — a reorder, an enable toggle, a tools
  // toggle — is expressed as the same pair (the new order, the complete flags
  // table) and lands here. That is not tidiness: the PUT replaces whole rows, so
  // a second call site that knew about only one of the two flags would write the
  // zero value over the other. A tools toggle followed by any drag would have
  // switched the tools straight back on.
  const commit = useCallback(
    async (orderedIds: string[], flagsById: Record<string, ProviderFlags>) => {
      const prefs = buildProviderPreferences(orderedIds, flagsById)
      const previous = useAgentProvidersStore.getState().providers

      // OPTIMISTIC. The rows are fully controlled off this list, so the switch
      // cannot move until the store does — and the next payload is built from
      // whatever this list says, so it has to say the truth before the response
      // lands or two quick toggles fight each other.
      //
      // The generation is claimed from the STORE, not from a ref local to this
      // component, and that is what fences reads as well as writes. The mount
      // refetch and the workspace-stream reseed are GETs that can be issued
      // before a write and answered after it, carrying the pre-write list; both
      // now decline to publish once this counter has moved. A ref here could
      // only ever sequence writes against writes, which is what let a stale GET
      // visibly flip a switch back on and hand the next write the flag it read.
      const seq = beginProviderWrite()
      publish(applyProviderPreferences(previous, orderedIds, flagsById))
      try {
        const resolved = await updateProviderPreferences(prefs)
        if (!isLatestProviderWrite(seq)) return // overtaken — a newer write owns the store
        publish(resolved)
      } catch {
        // A write that has already been superseded neither rolls back (it would
        // stomp the newer optimistic state) nor complains: its intent is carried
        // inside the newer payload, which PUTs the complete set and will report
        // its own failure if it fails too.
        if (!isLatestProviderWrite(seq)) return
        publish(previous)
        toast.error('Could not save providers', 'Crowbar could not reach the daemon — try again.')
      }
    },
    [publish],
  )

  const handleToggle = useCallback(
    (id: string, enabled: boolean) => {
      const flagsById = providerDisabledMap(providers)
      const orderedIds = providers.map((p) => p.id)
      // Flip ONE FIELD of one row; the other flag on that row (and every flag on
      // every other row) is carried through exactly as the list has it.
      void commit(orderedIds, {
        ...flagsById,
        [id]: { disabled: !enabled, mcpDisabled: flagsById[id]?.mcpDisabled ?? false },
      })
    },
    [providers, commit],
  )

  const handleToggleTools = useCallback(
    (id: string, enabled: boolean) => {
      const flagsById = providerDisabledMap(providers)
      const orderedIds = providers.map((p) => p.id)
      void commit(orderedIds, {
        ...flagsById,
        [id]: { disabled: flagsById[id]?.disabled ?? false, mcpDisabled: !enabled },
      })
    },
    [providers, commit],
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
      void commit(orderedIds, providerDisabledMap(providers))
    },
    [providers, commit],
  )

  const providerIds = useMemo(() => providers.map((p) => p.id), [providers])

  return (
    <div className="space-y-4">
      <Section title="Agents">
        {/* A Section's header is hidden when it is the first one in a tab
            (settings-section.tsx `first:[&>.settings-section-header]:hidden`),
            so this tab renders its own heading and intro in the BODY, mirroring
            that header's typography. Without them the group is a bare list of
            names that never says what an agent actually is. */}
        <div className="mb-2 px-1 py-1.5">
          <h3 className="ui-font ui-text-base font-medium text-foreground">Agents</h3>
          <p className="ui-font ui-text-sm text-muted-foreground">
            Agents are the coding CLIs Crowbar runs your chats on — Claude Code, Codex, and any
            others you add. Crowbar starts one for you when you open a chat, and each keeps its own
            session and login.
          </p>
        </div>
        <DefaultPermissionLevelSetting />
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
            Crowbar could not load the agent list — it could not reach the daemon. Reopen this tab
            to try again.
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
              New chat surface. Tools lets that agent use Crowbar itself — reading your workspaces
              and leaving review comments; turn it off and the agent still runs, with no way in.
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
                      onToggle={handleToggle}
                      onToggleTools={handleToggleTools}
                    />
                  ))}
                </div>
              </SortableContext>
            </DndContext>
          </>
        )}
      </Section>
      <ChatPresentationSetting />
    </div>
  )
}
