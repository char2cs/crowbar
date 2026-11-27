import { fitDisplayText, fitTitle } from './fit-title'
import type { LifecycleMessage } from './lifecycle-message'
import type { PaneChannel } from './pane-channel'
import type { PaneFrame } from './pane-frame'
import type { PaneParams } from './pane-params'
import type { OpenTarget } from './open-target'
import type { ReviewViewedFrame } from './review-viewed-frame'
import type { ViewedChange } from './viewed-change'
import type { PaneToast } from './pane-toast'
import type { SettingValues } from './setting-values'
import type { ShellFrame } from './shell-frame'
import type { ThemeTokens } from './theme-tokens'
import type { Breakpoint } from './breakpoint'
import type { FileDrop } from './file-drop'
import type { PairedHost } from './paired-host'
import {
  MAX_STATE_BYTES,
  REQUEST_TIMEOUT_MS,
  SDK_MAJOR,
  SDK_VERSION,
  SETTINGS_FRAME_VERSION,
} from './sdk-limits'
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

function settleWith(
  timer: ReturnType<typeof setTimeout>,
  settle: (value: unknown) => void,
): (value: unknown) => void {
  return (value) => {
    clearTimeout(timer)
    settle(value)
  }
}

function settling(timer: ReturnType<typeof setTimeout>, pending: Pending): Pending {
  return {
    resolve: settleWith(timer, pending.resolve),
    reject: settleWith(timer, pending.reject),
  }
}

const encoder = new TextEncoder()

export class PaneClient {
  private nextId = 1
  private readonly pending = new Map<number, Pending>()
  private readonly lifecycle = new Set<Listener<LifecycleMessage>>()
  private readonly themes = new Set<Listener<ThemeTokens>>()
  private readonly breakpoints = new Set<Listener<Breakpoint[]>>()
  private readonly chatZooms = new Set<Listener<number>>()
  private readonly saves = new Set<() => void>()
  private readonly groups = new Set<Listener<boolean>>()
  private readonly shellSettings = new Set<Listener<SettingValues>>()
  private readonly pairedHosts = new Set<Listener<PairedHost[]>>()
  private readonly events = new Map<string, EventSubscription>()
  private readonly reviewViewed = new Set<Listener<ReviewViewedFrame>>()
  private readonly reviewReveals = new Set<Listener<string>>()
  private readonly documentTexts = new Set<Listener<string>>()
  private readonly fileDrops = new Set<Listener<FileDrop>>()

  constructor(
    private readonly channel: PaneChannel,
    private readonly typeId: string,
  ) {
    channel.onmessage = (text) => this.receive(text)
  }

  register(): void {
    this.send({ kind: 'pane.register', typeId: this.typeId, sdkVersion: SDK_VERSION })
  }

  onLifecycle(listener: Listener<LifecycleMessage>): () => void {
    return this.listen(this.lifecycle, listener)
  }

  onTheme(listener: Listener<ThemeTokens>): () => void {
    return this.listen(this.themes, listener)
  }

  onBreakpoints(listener: Listener<Breakpoint[]>): () => void {
    return this.listen(this.breakpoints, listener)
  }

  onChatZoom(listener: Listener<number>): () => void {
    return this.listen(this.chatZooms, listener)
  }

  onGroup(listener: Listener<boolean>): () => void {
    return this.listen(this.groups, listener)
  }

  onSettings(listener: Listener<SettingValues>): () => void {
    return this.listen(this.shellSettings, listener)
  }

  onHosts(listener: Listener<PairedHost[]>): () => void {
    return this.listen(this.pairedHosts, listener)
  }

  onSave(listener: () => void): () => void {
    return this.listen(this.saves, listener)
  }

  onReviewViewed(listener: Listener<ReviewViewedFrame>): () => void {
    return this.listen(this.reviewViewed, listener)
  }

  onReviewReveal(listener: Listener<string>): () => void {
    return this.listen(this.reviewReveals, listener)
  }

  setReviewViewed(change: ViewedChange): void {
    this.send({ kind: 'review.setViewed', files: [...change.files], viewed: change.viewed })
  }

  onDocumentText(listener: Listener<string>): () => void {
    return this.listen(this.documentTexts, listener)
  }

  onFilesDropped(listener: Listener<FileDrop>): () => void {
    return this.listen(this.fileDrops, listener)
  }

  editDocument(text: string): void {
    this.send({ kind: 'document.edit', text })
  }

  showSource(): void {
    this.send({ kind: 'document.showSource' })
  }

  saveDocument(): void {
    this.send({ kind: 'document.save' })
  }

  markDirty(): void {
    this.send({ kind: 'pane.setDirty', dirty: true })
  }

  markClean(): void {
    this.send({ kind: 'pane.setDirty', dirty: false })
  }

  setTitle(title: string): void {
    this.send({ kind: 'pane.setTitle', title: fitTitle(title) })
  }

  setParams(params: PaneParams): void {
    this.send({ kind: 'pane.setParams', params })
  }

  setState(state: unknown): void {
    const size = encoder.encode(JSON.stringify(state)).length
    if (size > MAX_STATE_BYTES) {
      throw new RangeError(`pane state is ${size} bytes; the limit is ${MAX_STATE_BYTES}`)
    }
    this.send({ kind: 'pane.setState', state })
  }

  showToast(toast: PaneToast): void {
    const { description, ...rest } = toast
    const title = fitDisplayText(toast.title)
    const detail = description === undefined ? {} : { description: fitDisplayText(description) }
    this.send({ kind: 'shell.toast', ...rest, key: fitDisplayText(toast.key), title, ...detail })
  }

  closeToast(key: string): void {
    this.send({ kind: 'shell.toastClose', key: fitDisplayText(key) })
  }

  refuseOpen({ title, message }: { title: string; message: string }): void {
    if (!message) return this.send({ kind: 'pane.openRefused', message: fitDisplayText(title) })
    this.send({
      kind: 'pane.openRefused',
      title: fitDisplayText(title),
      message: fitDisplayText(message),
    })
  }

  open(typeId: string, params: PaneParams, target?: OpenTarget): void {
    this.send({ kind: 'pane.open', typeId, params, ...target })
  }

  runCommand(command: string): void {
    this.send({ kind: 'command.run', command })
  }

  request(method: string, params: unknown = null): Promise<unknown> {
    return this.call((id) => ({ kind: 'request', id, method, params }))
  }

  setSetting(key: string, value: unknown): void {
    this.send({ kind: 'settings.set', version: SETTINGS_FRAME_VERSION, key, value })
  }

  subscribe(event: string, listener: Listener<unknown>): () => void {
    const held = this.events.get(event) ?? this.openSubscription(event)
    held.listeners.add(listener)
    return () => this.release(event, held, listener)
  }

  private send(frame: PaneFrame): void {
    this.channel.post(JSON.stringify({ sdk: SDK_MAJOR, ...frame }))
  }

  private call(frame: (id: number) => PaneFrame): Promise<unknown> {
    return new Promise((resolve, reject) => {
      const id = this.nextId++
      const timer = setTimeout(() => this.expire(id, reject), REQUEST_TIMEOUT_MS)
      this.pending.set(id, settling(timer, { resolve, reject }))
      this.send(frame(id))
    })
  }

  private expire(id: number, reject: (error: unknown) => void): void {
    this.pending.delete(id)
    reject(new PaneRequestError({ code: 'Timeout', message: 'the shell did not answer in time' }))
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
    if (frame.kind === 'protocol.error')
      console.error('crowbar: the shell refused a frame', frame.message)
    if (frame.kind === 'pane.save') this.saves.forEach((listener) => listener())
    if (frame.kind === 'theme') this.themes.forEach((listener) => listener(frame.tokens))
    if (frame.kind === 'breakpoints')
      this.breakpoints.forEach((listener) => listener(frame.cleared))
    if (frame.kind === 'chat.zoom') this.chatZooms.forEach((listener) => listener(frame.percent))
    if (frame.kind === 'group') this.groups.forEach((listener) => listener(frame.chatFills))
    if (frame.kind === 'review.viewed') {
      const { workspace, viewed } = frame
      this.reviewViewed.forEach((listener) => listener({ workspace, viewed }))
    }
    if (frame.kind === 'review.reveal')
      this.reviewReveals.forEach((listener) => listener(frame.path))
    if (frame.kind === 'document.text')
      this.documentTexts.forEach((listener) => listener(frame.text))
    if (frame.kind === 'files.dropped') {
      const { paths, x, y } = frame
      this.fileDrops.forEach((listener) => listener({ paths, x, y }))
    }
    if (frame.kind === 'hosts') this.pairedHosts.forEach((listener) => listener(frame.hosts))
    if (frame.kind === 'settings' && frame.version === SETTINGS_FRAME_VERSION) {
      this.shellSettings.forEach((listener) => listener(frame.values))
    }
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
