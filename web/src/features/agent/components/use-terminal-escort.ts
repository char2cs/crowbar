import { useEffect, useEffectEvent, useLayoutEffect, useRef, useState, type RefObject } from 'react'
import { switchToNative, switchToTerminal } from '@/features/agent/api/agent-chat-commands'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import type {
  ChatPresentation,
  LandingChatPresentation,
} from '@/features/settings/lib/chat-presentation'
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
  isVisible: boolean
  presentation: ChatPresentation
  setPresentation: (next: ChatPresentation) => void
  splitting: boolean
  setSplitFocus: (focus: 'chat' | 'terminal') => void
  setReturnOffered: (offered: boolean) => void
  terminalApiRef: RefObject<{ focus: () => void } | null>
  /** No CLI runs on the chat, and startSession starts one. */
  dormant: boolean
  /** The chat's host does not answer, so asking it to fork a terminal view can only fail. */
  offline: boolean
  startSession: () => void
}

/** An unknown provider is hotswap: the synchronous behaviour every surface relies on. */
export function isHotswap(providers: AgentProvider[], providerId: string) {
  const provider = providers.find((candidate) => candidate.id === providerId)
  return { provider, hotswap: provider ? provider.hotswap === true : true }
}

// Moves between the chat and its terminal, only ever by the person's hand: a CLI waiting on its
// terminal is the banner's to signpost. 'sent': they followed that banner there, so once the wait
// clears Crowbar offers the way back; it never takes them.
export function useTerminalEscort(options: EscortOptions) {
  const { wsId, chatId, providers, activeProviderId, presentation, setPresentation } = options
  const { setReturnOffered, setSplitFocus, terminalApiRef, splitting } = options
  const escortRef = useRef<'none' | 'sent'>('none')

  // A hot-swap chat moves in the pane alone; the daemon records where, so a reload lands there.
  const showHotswap = (next: ChatPresentation): Promise<void> => {
    setPresentation(next)
    const record =
      next === 'terminal' ? switchToTerminal(wsId, chatId) : switchToNative(wsId, chatId)
    return record.then(
      () => undefined,
      (err: unknown) => console.warn('crowbar: record the chat surface', err),
    )
  }

  // The phone shows only the chat, so opening there a hot-swap chat recorded on its TUI brings
  // the chat's asks and compact back to the view the phone has. Only on opening: never a tug-of-war.
  const recordedKnown = options.recordedSurface !== null
  const takeChatOnPhone = useEffectEvent(() => {
    if (IS_MOBILE && options.isVisible && options.recordedSurface === 'terminal') {
      void showHotswap('chat')
    }
  })
  useEffect(() => {
    takeChatOnPhone()
  }, [chatId, options.isVisible, recordedKnown])

  // Only a provider that talks over its api connection asks the daemon to fork a
  // terminal view. The override covers a switch that resolved before a re-render.
  // Settles true once the terminal is the chat's recorded surface.
  const enterTerminal = async (providerIdOverride?: string): Promise<boolean> => {
    const { provider, hotswap } = isHotswap(providers, providerIdOverride ?? activeProviderId)
    if (hotswap) {
      await showHotswap('terminal')
      return true
    }
    if (options.onTerminalSurface) {
      setPresentation('terminal')
      return true
    }
    if (options.offline) return false
    try {
      await switchToTerminal(wsId, chatId)
      setPresentation('terminal')
      return true
    } catch (err: unknown) {
      const name = provider?.displayName ?? providerIdOverride ?? activeProviderId
      toastSpawnFailure(err, name, 'open the terminal view for')
      return false
    }
  }

  // Off the terminal surface: a non-hotswap provider drops the terminal view it forked first.
  const leaveTerminal = (next: ChatPresentation) => {
    const { provider, hotswap } = isHotswap(providers, activeProviderId)
    if (hotswap) {
      void showHotswap(next)
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
    if (nowWaiting) return
    const sent = escortRef.current === 'sent'
    escortRef.current = 'none'
    // The banner sent them here and the reason has gone: say so, and leave the move to them.
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- the offer belongs to one edge and is dismissible
    setReturnOffered(sent && presentation === 'terminal')
  })

  useEffect(() => {
    onWaitEdge(options.waiting)
  }, [options.waiting])

  // Picking a surface ends the banner's claim to offer the way back; split shows the
  // chat, so it counts as being back. A non-hotswap provider forks or drops its terminal view.
  const chooseSurface = (next: ChatPresentation) => {
    escortRef.current = 'none'
    if (next !== 'terminal') setReturnOffered(false)
    if (next === 'split') setSplitFocus(presentation === 'terminal' ? 'terminal' : 'chat')
    if (next === presentation) {
      setPresentation(next)
      return
    }
    if (isHotswap(providers, activeProviderId).hotswap) {
      void showHotswap(next)
      return
    }
    if (next === 'terminal') {
      void enterTerminal()
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

  // The chat's own way through: no claim on bringing anybody back. A CLI that is gone is
  // started once the terminal is its surface, so the way through never opens a dead terminal.
  const openTerminal = () => {
    const { dormant, startSession } = options
    if (splitting) {
      focusSplitTerminal()
      if (dormant) startSession()
      return
    }
    void enterTerminal().then((entered) => {
      if (entered && dormant) startSession()
    })
  }

  // The wait banner's button: Crowbar asked them to go, so it offers the way back.
  const openTerminalFromBanner = () => {
    if (splitting) {
      focusSplitTerminal()
      return
    }
    escortRef.current = 'sent'
    void enterTerminal()
  }

  // The handlers keep their identity across renders and call this render's moves: a
  // fresh one per render would be a fresh row action for every transcript row.
  const latest = useRef({ enterTerminal, chooseSurface, openTerminal, openTerminalFromBanner })
  useLayoutEffect(() => {
    latest.current = { enterTerminal, chooseSurface, openTerminal, openTerminalFromBanner }
  })
  const [handlers] = useState(() => ({
    enterTerminal: (providerId?: string) => latest.current.enterTerminal(providerId),
    chooseSurface: (next: ChatPresentation) => latest.current.chooseSurface(next),
    openTerminal: () => latest.current.openTerminal(),
    openTerminalFromBanner: () => latest.current.openTerminalFromBanner(),
  }))
  return handlers
}
