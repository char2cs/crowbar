import { fitTitle } from './fit-title'
import type { LifecycleMessage } from './lifecycle-message'
import type { PaneChannel } from './pane-channel'
import type { PaneFrame } from './pane-frame'
import type { PaneParams } from './pane-params'
import type { ShellFrame } from './shell-frame'
import type { ThemeTokens } from './theme-tokens'
import { MAX_STATE_BYTES, SDK_MAJOR, SDK_VERSION } from './sdk-limits'
import { PaneRequestError } from './pane-request-error'

type Listener<T> = (value: T) => void

interface Pending {
  resolve(value: unknown): void
  reject(error: unknown): void
}

interface EventSubscription {
  id: number
  listeners: Set<Listener<unknown>>
}

const encoder = new TextEncoder()

/** The SDK client of one pane page: registers the pane type with the shell, receives lifecycle, theme, event and response frames, and sends titles, state and requests. */
export class PaneClient {
  private nextId = 1
  private readonly pending = new Map<number, Pending>()
  private readonly lifecycle = new Set<Listener<LifecycleMessage>>()
  private readonly themes = new Set<Listener<ThemeTokens>>()
  private readonly events = new Map<string, EventSubscription>()
  private readonly settings = new Map<number, Listener<unknown>>()

  constructor(
    private readonly channel: PaneChannel,
    private readonly typeId: string,
  ) {
    channel.onmessage = (text) => this.receive(text)
  }

  /** Tells the shell which pane type this page implements; the shell answers with `mount`. */
  register(): void {
    this.send({ kind: 'pane.register', typeId: this.typeId, sdkVersion: SDK_VERSION })
  }

  /** Hears every lifecycle message; returns the stop. */
  onLifecycle(listener: Listener<LifecycleMessage>): () => void {
    return this.listen(this.lifecycle, listener)
  }

  /** Hears the design tokens on mount and whenever the theme changes; returns the stop. */
  onTheme(listener: Listener<ThemeTokens>): () => void {
    return this.listen(this.themes, listener)
  }

  /** Sets the pane's tab title, cutting it to the shell's limit. */
  setTitle(title: string): void {
    this.send({ kind: 'pane.setTitle', title: fitTitle(title) })
  }

  /** Replaces the persisted state; throws a RangeError when it is over the shell's state limit. */
  setState(state: unknown): void {
    const size = encoder.encode(JSON.stringify(state)).length
    if (size > MAX_STATE_BYTES) {
      throw new RangeError(`pane state is ${size} bytes; the limit is ${MAX_STATE_BYTES}`)
    }
    this.send({ kind: 'pane.setState', state })
  }

  /** Asks the shell to open another pane. */
  open(typeId: string, params: PaneParams): void {
    this.send({ kind: 'pane.open', typeId, params })
  }

  /** Runs a shell command by id. */
  runCommand(command: string): void {
    this.send({ kind: 'command.run', command })
  }

  /** Calls a method of the pane's own backend; rejects with a PaneRequestError on failure. */
  request(method: string, params: unknown = null): Promise<unknown> {
    return this.call((id) => ({ kind: 'request', id, method, params }))
  }

  /** Reads one setting. */
  getSetting(key: string): Promise<unknown> {
    return this.call((id) => ({ kind: 'settings.get', id, key }))
  }

  /** Follows a manifest-declared event, subscribing once per event name; returns the stop. */
  subscribe(event: string, listener: Listener<unknown>): () => void {
    const held = this.events.get(event) ?? this.openSubscription(event)
    held.listeners.add(listener)
    return () => this.release(event, held, listener)
  }

  /** Follows one setting, delivering its new value on each change; returns the stop. */
  subscribeSetting(key: string, listener: Listener<unknown>): () => void {
    const id = this.nextId++
    this.settings.set(id, listener)
    this.send({ kind: 'settings.subscribe', id, key })
    return () => {
      this.settings.delete(id)
      this.send({ kind: 'unsubscribe', id })
    }
  }

  private send(frame: PaneFrame): void {
    this.channel.post(JSON.stringify({ sdk: SDK_MAJOR, ...frame }))
  }

  private call(frame: (id: number) => PaneFrame): Promise<unknown> {
    return new Promise((resolve, reject) => {
      const id = this.nextId++
      this.pending.set(id, { resolve, reject })
      this.send(frame(id))
    })
  }

  private listen<T>(listeners: Set<Listener<T>>, listener: Listener<T>): () => void {
    listeners.add(listener)
    return () => void listeners.delete(listener)
  }

  private openSubscription(event: string): EventSubscription {
    const held = { id: this.nextId++, listeners: new Set<Listener<unknown>>() }
    this.events.set(event, held)
    this.send({ kind: 'subscribe', id: held.id, event })
    return held
  }

  private release(event: string, held: EventSubscription, listener: Listener<unknown>): void {
    held.listeners.delete(listener)
    const stillWanted = held.listeners.size > 0 || this.events.get(event) !== held
    if (stillWanted) return
    this.events.delete(event)
    this.send({ kind: 'unsubscribe', id: held.id })
  }

  private receive(text: string): void {
    let frame: ShellFrame
    try {
      frame = JSON.parse(text) as ShellFrame
    } catch (error) {
      console.error('crowbar: unreadable frame', error)
      return
    }
    this.dispatch(frame)
  }

  private dispatch(frame: ShellFrame): void {
    if (frame.kind === 'lifecycle') this.lifecycle.forEach((listener) => listener(frame))
    if (frame.kind === 'response') this.answer(frame)
    if (frame.kind === 'extension.event') {
      this.events.get(frame.name)?.listeners.forEach((listener) => listener(frame.payload))
    }
    if (frame.kind === 'settings.changed') this.settings.get(frame.id)?.(frame.value)
    if (frame.kind === 'theme') this.themes.forEach((listener) => listener(frame.tokens))
  }

  private answer(frame: Extract<ShellFrame, { kind: 'response' }>): void {
    const waiter = this.pending.get(frame.id)
    if (!waiter) return
    this.pending.delete(frame.id)
    if (!frame.error) {
      waiter.resolve(frame.result)
      return
    }
    waiter.reject(new PaneRequestError(frame.error))
  }
}
