import { parseLogFrame, type LogFrame } from './frames'

const CONSOLE_LOGS_PATH = '/v0/console/logs'

const RECONNECT_BASE_MS = 1000
const RECONNECT_MAX_MS = 30_000

export interface LogSocket {
  onopen: (() => void) | null
  onmessage: ((event: { data: string }) => void) | null
  onclose: (() => void) | null
  onerror: ((event: unknown) => void) | null
  close(): void
}

export type LogStreamState = 'idle' | 'connecting' | 'live' | 'reconnecting'

export interface LogStreamDeps {
  open: (path: string) => LogSocket
  cursor: () => number | null
  onFrame: (frame: LogFrame) => void
  onState: (state: LogStreamState) => void
  timers: {
    set: (fn: () => void, ms: number) => unknown
    clear: (handle: unknown) => void
  }
}

export interface LogStream {
  start(): void
  stop(): void
  running(): boolean
}

function streamPath(cursor: number | null): string {
  const query = new URLSearchParams({ level: 'debug' })
  if (cursor !== null) query.set('since', String(cursor))
  return `${CONSOLE_LOGS_PATH}?${query.toString()}`
}

export function createLogStream(deps: LogStreamDeps): LogStream {
  const { timers } = deps
  let active = false
  let socket: LogSocket | null = null
  let generation = 0
  let delay = RECONNECT_BASE_MS
  let timer: unknown = null
  let retrying = false

  function release(): void {
    generation++
    if (timer !== null) timers.clear(timer)
    timer = null
    const current = socket
    socket = null
    if (!current) return
    current.onopen = current.onmessage = current.onclose = current.onerror = null
    current.close()
  }

  function connect(): void {
    const mine = ++generation
    deps.onState(retrying ? 'reconnecting' : 'connecting')
    const next = deps.open(streamPath(deps.cursor()))
    socket = next

    next.onopen = () => {
      if (mine !== generation) return
      delay = RECONNECT_BASE_MS
      deps.onState('live')
    }
    next.onmessage = ({ data }) => {
      if (mine !== generation) return
      deps.onFrame(parseLogFrame(data))
    }
    next.onclose = () => {
      if (mine !== generation || !active) return
      socket = null
      retrying = true
      deps.onState('reconnecting')
      const wait = delay
      delay = Math.min(delay * 2, RECONNECT_MAX_MS)
      timer = timers.set(() => {
        timer = null
        if (active) connect()
      }, wait)
    }
    next.onerror = () => {}
  }

  return {
    start() {
      if (active) return
      active = true
      retrying = false
      delay = RECONNECT_BASE_MS
      connect()
    },
    stop() {
      if (!active) return
      active = false
      release()
      deps.onState('idle')
    },
    running: () => active,
  }
}
