import { useEffect, useEffectEvent, useRef, type RefObject } from 'react'
import { switchToNative, switchToTerminal } from '@/features/agent/api/agent-chat-commands'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

interface EscortOptions {
  wsId: string
  chatId: string
  providers: AgentProvider[]
  activeProviderId: string
  onTerminalSurface: boolean
  /** The CLI is blocked on a prompt only its terminal can answer. */
  waiting: boolean
  isActivePane: boolean
  isVisible: boolean
  presentation: ChatPresentation
  setPresentation: (next: ChatPresentation) => void
  splitting: boolean
  setSplitFocus: (focus: 'chat' | 'terminal') => void
  setReturnOffered: (offered: boolean) => void
  terminalApiRef: RefObject<{ focus: () => void } | null>
}

/** An unknown provider is hotswap: the synchronous behaviour every surface relies on. */
function isHotswap(providers: AgentProvider[], providerId: string) {
  const provider = providers.find((candidate) => candidate.id === providerId)
  return { provider, hotswap: provider ? provider.hotswap === true : true }
}

/**
 * Moving between the chat and its terminal, and escorting the user to the
 * terminal while the CLI waits there. 'sent' means Crowbar moved them and may
 * move them back; 'released' means they chose a surface since, so Crowbar only
 * offers the way back.
 */
export function useTerminalEscort(options: EscortOptions) {
  const { wsId, chatId, providers, activeProviderId, presentation, setPresentation } = options
  const { setReturnOffered, setSplitFocus, terminalApiRef, splitting } = options
  const escortRef = useRef<'none' | 'sent' | 'released'>('none')

  const userIsWatching = useEffectEvent(
    () => options.isActivePane && options.isVisible && getActiveWorkspaceId() === wsId,
  )

  // Only a provider that talks over its api connection asks the daemon to fork a
  // terminal view. The override covers a switch that resolved before a re-render.
  const enterTerminal = (providerIdOverride?: string) => {
    const { provider, hotswap } = isHotswap(providers, providerIdOverride ?? activeProviderId)
    if (hotswap || options.onTerminalSurface) {
      setPresentation('terminal')
      return
    }
    void (async () => {
      try {
        await switchToTerminal(wsId, chatId)
        setPresentation('terminal')
      } catch (err: unknown) {
        const name = provider?.displayName ?? providerIdOverride ?? activeProviderId
        toastSpawnFailure(err, name, 'open the terminal view for')
      }
    })()
  }

  // Off the terminal surface: a non-hotswap provider drops the terminal view it forked first.
  const leaveTerminal = (next: ChatPresentation) => {
    const { provider, hotswap } = isHotswap(providers, activeProviderId)
    if (hotswap) {
      setPresentation(next)
      return
    }
    void (async () => {
      try {
        await switchToNative(wsId, chatId)
        setPresentation(next)
      } catch (err: unknown) {
        // A refusal (a turn in flight) keeps the user on the terminal doing the work.
        toastSpawnFailure(err, provider?.displayName ?? activeProviderId, 'switch back to')
      }
    })()
  }

  // An effect event: it fires on the agent's edge, never on a focus or visibility change.
  const onWaitEdge = useEffectEvent((nowWaiting: boolean) => {
    if (nowWaiting) {
      // In split the terminal is already on screen, so there is no escort at all.
      if (presentation !== 'chat' || !userIsWatching()) return
      // react-doctor-disable-next-line no-adjust-state-on-prop-change -- a navigation on an edge: the surface outlives the value that moved it
      enterTerminal()
      escortRef.current = 'sent'
      return
    }
    const escort = escortRef.current
    escortRef.current = 'none'
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- the offer belongs to one edge and is dismissible
    setReturnOffered(false)
    if (escort === 'none') return
    if (escort === 'sent' && presentation === 'terminal' && userIsWatching()) {
      // react-doctor-disable-next-line no-adjust-state-on-prop-change -- see above
      leaveTerminal('chat')
      return
    }
    // They moved on, so Crowbar does not move them, but says why they are here.
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- see above
    if (presentation === 'terminal') setReturnOffered(true)
  })

  useEffect(() => {
    onWaitEdge(options.waiting)
  }, [options.waiting])

  // Picking a surface ends Crowbar's claim on it; split shows the chat, so it
  // counts as being back. A non-hotswap provider forks or drops its terminal view.
  const chooseSurface = (next: ChatPresentation) => {
    if (escortRef.current !== 'none') escortRef.current = next === 'terminal' ? 'released' : 'none'
    if (next !== 'terminal') setReturnOffered(false)
    if (next === 'split') setSplitFocus(presentation === 'terminal' ? 'terminal' : 'chat')
    if (next === presentation || isHotswap(providers, activeProviderId).hotswap) {
      setPresentation(next)
      return
    }
    if (next === 'terminal') {
      enterTerminal()
      return
    }
    if (presentation !== 'terminal') {
      setPresentation(next)
      return
    }
    leaveTerminal(next)
  }

  // In split the terminal is on screen already, so the caret goes there instead.
  const focusSplitTerminal = () => {
    setSplitFocus('terminal')
    terminalApiRef.current?.focus()
  }

  // The chat's own way through: no claim on bringing anybody back.
  const openTerminal = () => (splitting ? focusSplitTerminal() : enterTerminal())

  // The wait banner's button: Crowbar asked them to go, so it offers the way back.
  const openTerminalFromBanner = () => {
    if (splitting) {
      focusSplitTerminal()
      return
    }
    escortRef.current = 'released'
    enterTerminal()
  }

  return { enterTerminal, chooseSurface, openTerminal, openTerminalFromBanner }
}
