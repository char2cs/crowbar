import { type RefObject, useEffect, useRef } from 'react'

/** Listens on `target.current` (window when omitted). The latest handler is always called, so an
 *  inline arrow doesn't resubscribe on every render. */
export function useEventListener<K extends keyof DocumentEventMap>(
  eventName: K,
  handler: (event: DocumentEventMap[K]) => void,
  target?: RefObject<Document | HTMLElement | Window | null>,
): void {
  const handlerRef = useRef(handler)
  useEffect(() => {
    handlerRef.current = handler
  })
  useEffect(() => {
    const node = target ? target.current : window
    if (!node) return
    const listener = (event: Event) => handlerRef.current(event as DocumentEventMap[K])
    node.addEventListener(eventName, listener)
    return () => node.removeEventListener(eventName, listener)
  }, [eventName, target])
}
