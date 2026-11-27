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
  recordedSurface: LandingChatPresentation | null
  waiting: boolean
  isVisible: boolean
  presentation: ChatPresentation
  setPresentation: (next: ChatPresentation) => void
  splitting: boolean
  setSplitFocus: (focus: 'chat' | 'terminal') => void
  setReturnOffered: (offered: boolean) => void
  terminalApiRef: RefObject<{ focus: () => void } | null>
  dormant: boolean
  offline: boolean
  startSession: () => void
}

export function isHotswap(providers: AgentProvider[], providerId: string) {
  const provider = providers.find((candidate) => candidate.id === providerId)
  return { provider, hotswap: provider ? provider.hotswap === true : true }
}

export function useTerminalEscort(options: EscortOptions) {
  const { wsId, chatId, providers, activeProviderId, presentation, setPresentation } = options
  const { setReturnOffered, setSplitFocus, terminalApiRef, splitting } = options
  const escortRef = useRef<'none' | 'sent'>('none')

  const showHotswap = (next: ChatPresentation): Promise<void> => {
    setPresentation(next)
    const record =
      next === 'terminal' ? switchToTerminal(wsId, chatId) : switchToNative(wsId, chatId)
    return record.then(
      () => undefined,
      (err: unknown) => console.warn('crowbar: record the chat surface', err),
    )
  }

  const recordedKnown = options.recordedSurface !== null
  const takeChatOnPhone = useEffectEvent(() => {
    const chosen = presentation === 'terminal'
    if (IS_MOBILE && options.isVisible && options.recordedSurface === 'terminal' && !chosen) {
      void showHotswap('chat')
    }
  })
  useEffect(() => {
    takeChatOnPhone()
  }, [chatId, options.isVisible, recordedKnown])

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
        toastSpawnFailure(err, provider?.displayName ?? activeProviderId, 'switch back to')
      }
    })()
  }

  const onWaitEdge = useEffectEvent((nowWaiting: boolean) => {
    if (nowWaiting) return
    const sent = escortRef.current === 'sent'
    escortRef.current = 'none'
    // react-doctor-disable-next-line no-adjust-state-on-prop-change -- the offer belongs to one edge and is dismissible
    setReturnOffered(sent && presentation === 'terminal')
  })

  useEffect(() => {
    onWaitEdge(options.waiting)
  }, [options.waiting])

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

  const focusSplitTerminal = () => {
    setSplitFocus('terminal')
    terminalApiRef.current?.focus()
  }

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

  const openTerminalFromBanner = () => {
    if (splitting) {
      focusSplitTerminal()
      return
    }
    escortRef.current = 'sent'
    void enterTerminal()
  }

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
