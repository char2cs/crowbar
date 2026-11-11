import { useEffect, useEffectEvent, useRef } from 'react'
import {
  advanceTypeToFocus,
  EMPTY_TYPE_BUFFER,
  isFocusInEditable,
  type TypeBuffer,
} from '@/features/agent/chat/lib/type-to-focus'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

interface TypeToFocusOptions {
  wsId: string
  /** This chat is in front: its surface, its tab and its pane. */
  watched: boolean
  /** The composer has a field to type into right now. */
  canType: () => boolean
  onText: (text: string) => void
}

/** Typing anywhere in the chat lands in the composer. A window listener, since
 *  the box is not focused yet; a retained workspace stays mounted hidden, so the
 *  active workspace is asked inside the handler. */
export function useTypeToFocus({ wsId, watched, canType, onText }: TypeToFocusOptions) {
  const buffer = useRef<TypeBuffer>(EMPTY_TYPE_BUFFER)
  const onKey = useEffectEvent((event: KeyboardEvent) => {
    if (getActiveWorkspaceId() !== wsId) return
    if (isFocusInEditable(document.activeElement)) return
    if (!canType()) return
    const result = advanceTypeToFocus(buffer.current, event, Date.now())
    buffer.current = result.buffer
    if (result.action !== 'redirect') return
    event.preventDefault()
    event.stopPropagation()
    onText(result.text)
  })
  useEffect(() => {
    if (!watched) return
    buffer.current = EMPTY_TYPE_BUFFER
    const listener = (event: KeyboardEvent) => onKey(event)
    window.addEventListener('keydown', listener, true)
    return () => window.removeEventListener('keydown', listener, true)
  }, [watched])
}
