import { useEffect, useEffectEvent, useRef, type RefObject } from 'react'
import { switchToNative, switchToTerminal } from '@/features/agent/api/agent-chat-commands'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import type {
  ChatPresentation,
  LandingChatPresentation,
} from '@/features/settings/lib/chat-presentation'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { IS_MOBILE } from '@/utils/platform'

interface EscortOptions {
  wsId: string
  chatId: string
  providers: AgentProvider[]
  activeProviderId: string
  onTerminalSurface: boolean
  /** Where the daemon recorded a hot-swap chat was last shown; null without hot-swap. */
  recordedSurface: LandingChatPresentation | null
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
export function isHotswap(providers: AgentProvider[], providerId: string) {
  const provider = providers.find((candidate) => candidate.id === providerId)
  return { provider, hotswap: provider ? provider.hotswap === true : true }
}

// Moves between the chat and its terminal, escorting the user there while the CLI waits. 'sent':
// Crowbar moved them and may move them back; 'released': they chose a surface since, so Crowbar
// only offers the way back.
export function useTerminalEscort(options: EscortOptions) {
  const { wsId, chatId, providers, activeProviderId, presentation, setPresentation } = options
  const { setReturnOffered, setSplitFocus, terminalApiRef, splitting } = options
  const escortRef = useRef<'none' | 'sent' | 'released'>('none')

  // A hot-swap chat moves in the pane alone; the daemon records where, so a reload lands there.
  const showHotswap = (next: ChatPresentation) => {
    setPresentation(next)
    const record =
      next === 'terminal' ? switchToTerminal(wsId, chatId) : switchToNative(wsId, chatId)
    void record.catch((err: unknown) => console.warn('crowbar: record the chat surface', err))
  }

  // The phone shows only the chat, so opening there a hot-swap chat recorded on its TUI brings
  // the chat's asks and compact back to the view the phone has. Only on opening: never a tug-of-war.
  const recordedKnown = options.recordedSurface !== null
  const takeChatOnPhone = useEffectEvent(() => {
    if (IS_MOBILE && options.isVisible && options.recordedSurface === 'terminal') {
      showHotswap('chat')
    }
  })
  useEffect(() => takeChatOnPhone(), [chatId, options.isVisible, recordedKnown])

  const userIsWatching = useEffectEvent(
    () => options.isActivePane && options.isVisible && getActiveWorkspaceId() === wsId,
  )

  // Only a provider that talks over its api connection asks the daemon to fork a
  // terminal view. The override covers a switch that resolved before a re-render.
  const enterTerminal = (providerIdOverride?: string) => {
    const { provider, hotswap } = isHotswap(providers, providerIdOverride ?? activeProviderId)
    if (hotswap) {
      showHotswap('terminal')
      return
    }
    if (options.onTerminalSurface) {
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
      showHotswap(next)
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
    if (next === presentation) {
      setPresentation(next)
      return
    }
    if (isHotswap(providers, activeProviderId).hotswap) {
      showHotswap(next)
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
