import { report } from '@/lib/errors/report'
import { wsManager } from '@/core/stream/ws-manager'
import { qualify } from '@/lib/host-path'
import { decodeFrame, FrameError, type Frame } from '@/core/stream/frames'
import type { Problem } from '@/lib/errors/problem'
import { UserFacingError } from '@/lib/errors/user-facing-error'

export interface ScopeKindHandler {
  since?(hostId: string, key: string): number
  bytes?(hostId: string, key: string): number
  subscribed?(hostId: string, scope: string): void
  frame(hostId: string, f: Frame): void
  epoch?(hostId: string, epoch: string): void
  forget?(hostId: string): void
}

export interface ScopeWatch {
  synced?(): void
  refused?(problem: Problem | undefined): void
  failed?(times: number): void
}

type SignalFrame = Extract<Frame, { t: 'signal' }>

const PROJECTS = 'projects'

function kindOf(scope: string): string {
  const colon = scope.indexOf(':')
  return colon < 0 ? scope : scope.slice(0, colon)
}

function keyOf(scope: string): string {
  const colon = scope.indexOf(':')
  return colon < 0 ? '' : scope.slice(colon + 1)
}

function listen<T>(registry: Map<string, Set<T>>, scope: string, listener: T): () => void {
  const set = registry.get(scope) ?? new Set<T>()
  registry.set(scope, set.add(listener))
  return () => {
    set.delete(listener)
    if (set.size === 0 && registry.get(scope) === set) registry.delete(scope)
  }
}

export class HostStream {
  private readonly endpoint: string
  private readonly holds = new Map<string, number>()
  private readonly watches = new Map<string, Set<ScopeWatch>>()
  private readonly signals = new Map<string, Set<(f: SignalFrame) => void>>()
  private readonly current = new Set<string>()
  private readonly awaiting = new Set<string>()
  private readonly failures = new Map<string, number>()
  private ready = false
  private stopSocket: (() => void) | null = null
  epoch: string | undefined

  constructor(
    readonly hostId: string,
    private readonly handlers: ReadonlyMap<string, ScopeKindHandler>,
  ) {
    this.endpoint = qualify(hostId, '/v0/stream')
  }

  start(): void {
    if (this.stopSocket) return
    const stopDrops = wsManager.onDrop(this.endpoint, () => {
      this.ready = false
      this.current.clear()
      for (const scope of [...this.awaiting]) this.snapshotFailed(scope)
    })
    const stop = wsManager.subscribe(this.endpoint, (raw) => this.onMessage(raw))
    const letGoOfProjects = this.hold(PROJECTS)
    this.stopSocket = () => {
      letGoOfProjects()
      this.ready = false
      stopDrops()
      stop()
    }
  }

  stop(): void {
    this.stopSocket?.()
    this.stopSocket = null
  }

  opened(): Promise<void> {
    return wsManager.opened(this.endpoint)
  }

  private handler(scope: string): ScopeKindHandler | undefined {
    return this.handlers.get(kindOf(scope))
  }

  private send(req: { op: string; scope: string; since?: number; bytes?: number }): void {
    wsManager.send(this.endpoint, req)
  }

  private sub(scope: string): void {
    this.current.delete(scope)
    this.awaiting.add(scope)
    const h = this.handler(scope)
    h?.subscribed?.(this.hostId, scope)
    const since = h?.since?.(this.hostId, keyOf(scope))
    const bytes = h?.bytes?.(this.hostId, keyOf(scope))
    this.send({ op: 'sub', scope, ...(since && { since }), ...(bytes && { bytes }) })
  }

  hold(scope: string): () => void {
    const count = this.holds.get(scope) ?? 0
    this.holds.set(scope, count + 1)
    if (count === 0 && this.ready) this.sub(scope)
    let released = false
    return () => {
      if (released) return
      released = true
      const left = (this.holds.get(scope) ?? 0) - 1
      if (left > 0) {
        this.holds.set(scope, left)
        return
      }
      if (!this.holds.delete(scope)) return
      this.current.delete(scope)
      this.awaiting.delete(scope)
      this.failures.delete(scope)
      if (this.ready) this.send({ op: 'unsub', scope })
    }
  }

  resubscribe(scope: string): void {
    if (!this.holds.has(scope) || !this.ready) return
    this.current.delete(scope)
    this.send({ op: 'unsub', scope })
    this.sub(scope)
  }

  holding(scope: string): boolean {
    return this.holds.has(scope)
  }

  watch(scope: string, watch: ScopeWatch): () => void {
    const stop = listen(this.watches, scope, watch)
    if (this.current.has(scope)) watch.synced?.()
    return stop
  }

  onSignal(scope: string, kind: string, cb: (data: unknown) => void): () => void {
    return listen(this.signals, scope, (f: SignalFrame) => {
      if (f.kind === kind) cb(f.data)
    })
  }

  private snapshotFailed(scope: string): void {
    this.awaiting.delete(scope)
    const times = (this.failures.get(scope) ?? 0) + 1
    this.failures.set(scope, times)
    for (const w of [...(this.watches.get(scope) ?? [])]) w.failed?.(times)
  }

  private dropScope(scope: string, why: unknown, problem?: Problem): void {
    const told = [...(this.watches.get(scope) ?? [])].filter((w) => w.refused)
    if (told.length === 0) report(why, { action: `Stream ${scope}` })
    this.current.delete(scope)
    if (this.holds.delete(scope) && this.ready) this.send({ op: 'unsub', scope })
    for (const w of told) w.refused?.(problem)
  }

  private onReady(next: string): void {
    this.ready = true
    if (next !== this.epoch) {
      this.epoch = next
      for (const h of this.handlers.values()) h.epoch?.(this.hostId, next)
    }
    for (const scope of this.holds.keys()) this.sub(scope)
  }

  private onFrame(f: Frame): void {
    if (f.t === 'ready') return this.onReady(f.epoch)
    if (f.t === 'error') {
      return report(new UserFacingError(f.problem.message), { action: 'Stream' })
    }
    if (!this.holds.has(f.scope)) return
    this.handler(f.scope)?.frame(this.hostId, f)
    if (f.t === 'signal') for (const cb of [...(this.signals.get(f.scope) ?? [])]) cb(f)
    if (f.t === 'synced') {
      this.current.add(f.scope)
      this.awaiting.delete(f.scope)
      this.failures.delete(f.scope)
      for (const w of [...(this.watches.get(f.scope) ?? [])]) w.synced?.()
    }
    if (f.t !== 'closed') return
    if (f.reason === 'resync') this.sub(f.scope)
    else {
      const why = new UserFacingError(f.problem?.message ?? `${f.scope} was ${f.reason}`)
      this.dropScope(f.scope, why, f.problem)
    }
  }

  private askAgain(scope: string, err: unknown): void {
    report(err, { action: `Stream ${scope}` })
    if (this.awaiting.has(scope)) this.snapshotFailed(scope)
    if ((this.failures.get(scope) ?? 0) < 2) this.resubscribe(scope)
  }

  private onMessage(raw: unknown): void {
    if ((raw as { reconnected?: unknown } | null)?.reconnected === true) return
    let f: Frame
    try {
      f = decodeFrame(raw)
    } catch (err) {
      const scope = err instanceof FrameError ? err.scope : undefined
      if (scope === PROJECTS && this.holds.has(scope)) this.askAgain(scope, err)
      else if (scope && this.holds.has(scope)) this.dropScope(scope, err)
      else if (!scope) report(err, { action: 'Stream' })
      return
    }
    this.onFrame(f)
  }
}
