import {
  memo,
  type RefObject,
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
} from 'react'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { useEditorLayout } from '@/features/editor/hooks/use-layout'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { useEditorUIStore } from '@/features/editor/stores/ui-store'
import { extensionRegistry } from '@/extensions/registry/extension-registry'
import type { EditorModelPositionResolver } from '../view-model/view-layout'
import { LspClient } from './lsp-client'

interface SignatureInfo {
  label: string
  documentation?: { kind: string; value: string } | string
  parameters?: {
    label: string | [number, number]
    documentation?: { kind: string; value: string } | string
  }[]
  activeParameter?: number
}

interface SignatureHelpResult {
  signatures: SignatureInfo[]
  activeSignature?: number
  activeParameter?: number
}

const DEFAULT_TRIGGER_CHARS = ['(', ',']

interface SignatureHelpTooltipProps {
  editorRef: RefObject<HTMLDivElement | null>
  filePath: string | undefined
  resolveModelPosition?: EditorModelPositionResolver
}

// Memoized so a PaneLspLayer reconcile (find toggle, code-lens fetch, rename
// state, zoom/settings, the post-swap rich-services gate) does NOT re-render this
// tooltip when it has nothing to show. Its inputs are a stable ref object
// (`editorRef`), a stable resolver (`resolveModelPosition`, `useCallback([])` in
// EditorSurface), and `filePath` — which only changes on a buffer switch. It owns
// its own visibility (cursor/typing subscriptions → `signatureHelp` state).
const SignatureHelpTooltipImpl = ({
  editorRef,
  filePath,
  resolveModelPosition,
}: SignatureHelpTooltipProps) => {
  const [signatureHelp, setSignatureHelp] = useState<SignatureHelpResult | null>(null)
  const { charWidth, lineHeight } = useEditorLayout()
  const requestIdRef = useRef(0)
  const scrollOffsetRef = useRef({ top: 0, left: 0 })
  const cursorPositionRef = useRef(useEditorStateStore.getState().cursorPosition)
  const signatureHelpRef = useRef<SignatureHelpResult | null>(null)
  const [tooltipCursorPosition, setTooltipCursorPosition] = useState(cursorPositionRef.current)
  const [triggerCharacters, setTriggerCharacters] = useState(DEFAULT_TRIGGER_CHARS)

  useEffect(() => {
    signatureHelpRef.current = signatureHelp
  }, [signatureHelp])

  // Track scroll position
  useEffect(() => {
    const textarea = editorRef.current?.querySelector('textarea')
    if (!textarea) return

    const handleScroll = () => {
      scrollOffsetRef.current = {
        top: textarea.scrollTop,
        left: textarea.scrollLeft,
      }
    }

    textarea.addEventListener('scroll', handleScroll)
    handleScroll()
    return () => textarea.removeEventListener('scroll', handleScroll)
  }, [editorRef, filePath])

  useEffect(() => {
    if (!filePath || !extensionRegistry.isLspSupported(filePath)) {
      setTriggerCharacters(DEFAULT_TRIGGER_CHARS)
      return
    }

    let cancelled = false
    const lspClient = LspClient.getInstance()

    lspClient.getSignatureTriggerCharacters(filePath).then((characters) => {
      if (cancelled) return
      setTriggerCharacters(characters.length > 0 ? characters : DEFAULT_TRIGGER_CHARS)
    })

    return () => {
      cancelled = true
    }
  }, [filePath])

  const fetchSignatureHelp = useCallback(async () => {
    if (!filePath || !extensionRegistry.isLspSupported(filePath)) {
      setSignatureHelp(null)
      return
    }

    const id = ++requestIdRef.current
    const lspClient = LspClient.getInstance()
    const cursorPosition = cursorPositionRef.current
    const result = await lspClient.getSignatureHelp(
      filePath,
      cursorPosition.line,
      cursorPosition.column,
    )

    if (id !== requestIdRef.current) return

    if (result && result.signatures.length > 0) {
      setSignatureHelp(result)
    } else {
      setSignatureHelp(null)
    }
  }, [filePath])

  useEffect(() => {
    let previousOffset = useEditorStateStore.getState().cursorPosition.offset
    let refreshTimeout: ReturnType<typeof globalThis.setTimeout> | null = null

    const unsubscribe = useEditorStateStore.subscribe((state) => {
      const nextPosition = state.cursorPosition
      cursorPositionRef.current = nextPosition

      if (nextPosition.offset === previousOffset) {
        return
      }
      previousOffset = nextPosition.offset

      if (!signatureHelpRef.current) {
        return
      }

      setTooltipCursorPosition(nextPosition)
      if (refreshTimeout !== null) {
        globalThis.clearTimeout(refreshTimeout)
      }
      refreshTimeout = globalThis.setTimeout(() => {
        void fetchSignatureHelp()
        refreshTimeout = null
      }, 100)
    })

    return () => {
      unsubscribe()
      if (refreshTimeout !== null) {
        globalThis.clearTimeout(refreshTimeout)
      }
    }
  }, [fetchSignatureHelp])

  useEffect(() => {
    const handleTriggerSignatureHelp = () => {
      setTooltipCursorPosition(cursorPositionRef.current)
      void fetchSignatureHelp()
    }

    window.addEventListener('editor-trigger-signature-help', handleTriggerSignatureHelp)
    return () =>
      window.removeEventListener('editor-trigger-signature-help', handleTriggerSignatureHelp)
  }, [fetchSignatureHelp])

  // Read the latest fetchSignatureHelp/triggerCharacters via an Effect Event so
  // the store subscription is established once instead of re-subscribing every
  // time those change.
  const onEditorInput = useEffectEvent(() => {
    // Check if the character just typed is a trigger character
    const textarea = editorRef.current?.querySelector('textarea')
    if (!textarea) return

    const content = textarea.value
    const offset = cursorPositionRef.current.offset
    if (offset <= 0) return

    const charBefore = content[offset - 1]
    if (triggerCharacters.includes(charBefore)) {
      setTooltipCursorPosition(cursorPositionRef.current)
      void fetchSignatureHelp()
    } else if (charBefore === ')') {
      setSignatureHelp(null)
    }
  })

  // Trigger on typing
  useEffect(() => {
    let lastInputTimestamp = useEditorUIStore.getState().lastInputTimestamp

    const unsubscribe = useEditorUIStore.subscribe((state) => {
      if (state.lastInputTimestamp === 0 || state.lastInputTimestamp === lastInputTimestamp) {
        return
      }

      lastInputTimestamp = state.lastInputTimestamp
      onEditorInput()
    })

    return unsubscribe
  }, [])

  const position = useMemo(() => {
    const cursorPosition = tooltipCursorPosition
    const resolvedPosition = resolveModelPosition?.(cursorPosition.line, cursorPosition.column)

    return {
      top:
        (resolvedPosition?.top ??
          EDITOR_CONSTANTS.EDITOR_PADDING_TOP + cursorPosition.line * lineHeight) -
        scrollOffsetRef.current.top -
        lineHeight -
        8,
      left:
        (resolvedPosition?.left ??
          EDITOR_CONSTANTS.EDITOR_PADDING_LEFT + cursorPosition.column * charWidth) -
        scrollOffsetRef.current.left,
    }
  }, [tooltipCursorPosition, charWidth, lineHeight, resolveModelPosition])

  if (!signatureHelp || signatureHelp.signatures.length === 0) return null

  const activeIdx = signatureHelp.activeSignature ?? 0
  const signature = signatureHelp.signatures[activeIdx]
  if (!signature) return null

  const activeParam = signatureHelp.activeParameter ?? signature.activeParameter ?? 0

  // Render signature label with active parameter highlighted
  const renderLabel = () => {
    if (!signature.parameters || signature.parameters.length === 0) {
      return <span>{signature.label}</span>
    }

    const param = signature.parameters[activeParam]
    if (!param) return <span>{signature.label}</span>

    if (Array.isArray(param.label)) {
      const [start, end] = param.label
      return (
        <span>
          {signature.label.slice(0, start)}
          <span className="font-bold text-secondary">{signature.label.slice(start, end)}</span>
          {signature.label.slice(end)}
        </span>
      )
    }

    // String label — find it in the signature label
    const paramStr = param.label
    const idx = signature.label.indexOf(paramStr)
    if (idx === -1) return <span>{signature.label}</span>

    return (
      <span>
        {signature.label.slice(0, idx)}
        <span className="font-bold text-secondary">{paramStr}</span>
        {signature.label.slice(idx + paramStr.length)}
      </span>
    )
  }

  return (
    <div
      className="absolute z-50 max-w-md rounded-md border border-border/70 bg-card px-2.5 py-1.5 shadow-lg"
      style={{
        top: `${Math.max(4, position.top)}px`,
        left: `${Math.max(EDITOR_CONSTANTS.EDITOR_PADDING_LEFT, position.left)}px`,
      }}
    >
      <div className="ui-font ui-text-sm editor-font text-foreground">{renderLabel()}</div>
    </div>
  )
}

export const SignatureHelpTooltip = memo(SignatureHelpTooltipImpl)
SignatureHelpTooltip.displayName = 'SignatureHelpTooltip'
