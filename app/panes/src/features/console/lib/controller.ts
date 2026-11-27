import type { LogFrame } from './frames'
import { createLogStream, type LogSocket, type LogStreamState } from './log-stream'

const FLUSH_MS = 50

interface ConsoleView {
  open: boolean
  cursor: number | null
  setStream: (state: LogStreamState) => void
  ingest: (frames: readonly LogFrame[]) => void
}

interface ConsoleStore {
  getState: () => ConsoleView
  subscribe: (listener: (state: ConsoleView, previous: ConsoleView) => void) => () => void
}

export interface ControllerDeps {
  store: ConsoleStore
  open: (path: string) => LogSocket
  timers: {
    set: (fn: () => void, ms: number) => unknown
    clear: (handle: unknown) => void
  }
}

export interface ConsoleController {
  dispose(): void
}

export function createConsoleController(deps: ControllerDeps): ConsoleController {
  const { store, timers } = deps
  let queue: LogFrame[] = []
  let replay: LogFrame[] | null = null
  let flushTimer: unknown = null

  function flush(): void {
    if (flushTimer !== null) timers.clear(flushTimer)
    flushTimer = null
    if (queue.length === 0) return
    const frames = queue
    queue = []
    store.getState().ingest(frames)
  }

  const stream = createLogStream({
    open: deps.open,
    cursor: () => store.getState().cursor,
    onFrame: (frame) => {
      if (replay === null) {
        queue.push(frame)
      } else {
        replay.push(frame)
        if (frame.type === 'ready') {
          queue.push(...replay)
          replay = null
        }
      }
      if (queue.length > 0) flushTimer ??= timers.set(flush, FLUSH_MS)
    },
    onState: (state) => {
      if (state === 'connecting' || state === 'reconnecting') replay = []
      if (state === 'idle') replay = null
      store.getState().setStream(state)
    },
    timers,
  })

  function reconcile(): void {
    const wanted = store.getState().open
    if (wanted && !stream.running()) stream.start()
    if (!wanted && stream.running()) {
      flush()
      stream.stop()
    }
  }

  const unsubscribe = store.subscribe((s, prev) => {
    if (s.open !== prev.open) reconcile()
  })
  reconcile()

  return {
    dispose() {
      unsubscribe()
      flush()
      stream.stop()
    },
  }
}
