import { useCallback, useEffect, useRef } from 'react'
import { recordInputTape } from '@/features/terminal/utils/input-tape'

interface TerminalWriteBufferOptions {
  getConnectionId: () => string | null
  writeChunk: (connectionId: string, data: string) => Promise<void>
}

export function useTerminalWriteBuffer({
  getConnectionId,
  writeChunk,
}: TerminalWriteBufferOptions) {
  const queueRef = useRef('')
  const flushingRef = useRef(false)
  const getConnectionIdRef = useRef(getConnectionId)
  const writeChunkRef = useRef(writeChunk)

  getConnectionIdRef.current = getConnectionId
  writeChunkRef.current = writeChunk

  const flush = useCallback(async () => {
    if (flushingRef.current) return

    while (queueRef.current) {
      const connectionId = getConnectionIdRef.current()
      const data = queueRef.current
      if (!connectionId) {
        // The transport is momentarily absent (a re-attach). The bytes stay
        // queued; record that they have NOT gone out so a later diagnosis can
        // tell a stalled write apart from a delivered one.
        recordInputTape('send', 'deferred:no-connection', data)
        return
      }

      queueRef.current = ''
      flushingRef.current = true
      try {
        recordInputTape('send', 'chunk', data, { connectionId })
        await writeChunkRef.current(connectionId, data)
      } catch (error) {
        // Requeued in front of anything typed since, so order is preserved —
        // but if the failed write DID reach the PTY this is where a duplicate
        // would be born, so it is recorded explicitly.
        recordInputTape('send', 'requeued-after-error', data, {
          error: String(error).slice(0, 120),
        })
        queueRef.current = data + queueRef.current
        break
      } finally {
        flushingRef.current = false
      }
    }
  }, [])

  const write = useCallback(
    (data: string, origin = 'unknown') => {
      if (!data) return
      recordInputTape('write', origin, data)
      queueRef.current += data
      void flush()
    },
    [flush],
  )

  useEffect(() => {
    return () => {
      void flush()
    }
  }, [flush])

  return { write, flush }
}
