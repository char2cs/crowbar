import type { LogFrame } from './frames'
import { createLogStream, type LogSocket, type LogStreamState } from './log-stream'

/** Frames from the stream are folded into the store in batches, not one render each. */
const FLUSH_MS = 50

/** What the controller reads of the console store and the actions it takes on it. */
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

/**
 * Keeps the log stream open while the console is, and folds what arrives into
 * the store. The replay of each connection is held back until its `ready`: a
 * `reset` there means the replay replaces what is shown, which cannot be decided
 * before the whole replay is in hand. It holds no state beyond handles.
 */
export function createConsoleController(deps: ControllerDeps): ConsoleController {
  const { store, timers } = deps
  let queue: LogFrame[] = []
  /** The replay being held; null once it is over, when frames pass straight through. */
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
      // Every new socket begins with a replay, and its frames can beat `onopen`,
      // so the hold starts at the attempt, not at the open.
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
