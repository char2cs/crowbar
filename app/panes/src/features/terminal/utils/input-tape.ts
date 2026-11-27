const MAX_ENTRIES = 20_000

export type InputTapeKind = 'dom' | 'write' | 'send'

interface InputTapeEntry {
  t: number
  kind: InputTapeKind
  label: string
  data?: string
  meta?: Record<string, string | number | boolean>
}

const entries: InputTapeEntry[] = []
let dropped = 0

function readable(data: string): string {
  // eslint-disable-next-line no-control-regex
  return data.replace(/[\u0000-\u001f\u007f]/g, (ch) => {
    if (ch === '\u001b') return '\\e'
    if (ch === '\r') return '\\r'
    if (ch === '\n') return '\\n'
    if (ch === '\t') return '\\t'
    return `\\x${ch.charCodeAt(0).toString(16).padStart(2, '0')}`
  })
}

export function recordInputTape(
  kind: InputTapeKind,
  label: string,
  data?: string,
  meta?: Record<string, string | number | boolean>,
): void {
  if (entries.length >= MAX_ENTRIES) {
    entries.shift()
    dropped++
  }
  entries.push({
    t: Math.round(typeof performance !== 'undefined' ? performance.now() : 0),
    kind,
    label,
    ...(data === undefined ? {} : { data: readable(data) }),
    ...(meta === undefined ? {} : { meta }),
  })
}

interface InputTapeDump {
  dropped: number
  entries: InputTapeEntry[]
}

function dumpInputTape(): InputTapeDump {
  return { dropped, entries: entries.slice() }
}

function clearInputTape(): void {
  entries.length = 0
  dropped = 0
}

export function observeInputEvents(textarea: HTMLTextAreaElement): () => void {
  const onKey = (event: Event) => {
    const e = event as KeyboardEvent
    recordInputTape('dom', e.type, e.key, {
      code: e.code,
      keyCode: e.keyCode,
      shift: e.shiftKey,
      alt: e.altKey,
      ctrl: e.ctrlKey,
      meta: e.metaKey,
      repeat: e.repeat,
      composing: e.isComposing,
      value: (event.currentTarget as HTMLTextAreaElement).value,
    })
  }
  const onInput = (event: Event) => {
    const e = event as InputEvent
    recordInputTape('dom', e.type, e.data ?? '', {
      inputType: e.inputType,
      composing: e.isComposing,
      value: (event.currentTarget as HTMLTextAreaElement).value,
    })
  }
  const onComposition = (event: Event) => {
    const e = event as CompositionEvent
    recordInputTape('dom', e.type, e.data ?? '', {
      value: (event.currentTarget as HTMLTextAreaElement).value,
    })
  }

  const keyTypes = ['keydown', 'keypress', 'keyup']
  const inputTypes = ['beforeinput', 'input']
  const compositionTypes = ['compositionstart', 'compositionupdate', 'compositionend']

  for (const type of keyTypes) textarea.addEventListener(type, onKey, true)
  for (const type of inputTypes) textarea.addEventListener(type, onInput, true)
  for (const type of compositionTypes) textarea.addEventListener(type, onComposition, true)

  return () => {
    for (const type of keyTypes) textarea.removeEventListener(type, onKey, true)
    for (const type of inputTypes) textarea.removeEventListener(type, onInput, true)
    for (const type of compositionTypes) textarea.removeEventListener(type, onComposition, true)
  }
}

export function installInputTapeGlobal(): void {
  if (typeof window === 'undefined') return
  const host = window as unknown as Record<string, unknown>
  host.__crowbarInputTape = (options?: { copy?: boolean; clear?: boolean }) => {
    const dump = dumpInputTape()
    if (options?.copy) void navigator.clipboard?.writeText(JSON.stringify(dump, null, 2))
    if (options?.clear) clearInputTape()
    return dump
  }
}
