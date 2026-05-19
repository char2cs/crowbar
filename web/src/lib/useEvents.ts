import { useEffect, useRef } from 'react'
import { createEventSource } from './transport'

export function useEvents(eventType: string, handler: (data: unknown) => void): void {
  const handlerRef = useRef(handler)
  handlerRef.current = handler

  useEffect(() => {
    const es = createEventSource()

    const listener = (e: MessageEvent) => {
      try {
        handlerRef.current(JSON.parse(e.data))
      } catch {
        handlerRef.current(e.data)
      }
    }

    es.addEventListener(eventType, listener)
    return () => {
      es.removeEventListener(eventType, listener)
      es.close()
    }
  }, [eventType])
}
