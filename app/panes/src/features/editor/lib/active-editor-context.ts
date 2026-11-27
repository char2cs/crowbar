export interface ActiveEditorContext {
  paneId: string
  uri: string
  filePath: string
  model?: unknown
  editor?: unknown
}

export interface ActiveEditorRegistry {
  set(paneId: string, ctx: ActiveEditorContext): void
  get(paneId: string): ActiveEditorContext | undefined
  subscribe(paneId: string, cb: (ctx: ActiveEditorContext | undefined) => void): () => void
  clear(paneId: string): void
  clearIfActive(paneId: string, uri: string): void
}

type Listener = (ctx: ActiveEditorContext | undefined) => void

interface PaneState {
  ctx: ActiveEditorContext | undefined
  listeners: Set<Listener>
}

function createActiveEditorRegistry(): ActiveEditorRegistry {
  const panes = new Map<string, PaneState>()

  function getOrCreate(paneId: string): PaneState {
    let state = panes.get(paneId)
    if (!state) {
      state = { ctx: undefined, listeners: new Set() }
      panes.set(paneId, state)
    }
    return state
  }

  function notify(state: PaneState, ctx: ActiveEditorContext | undefined): void {
    for (const cb of state.listeners) {
      cb(ctx)
    }
  }

  return {
    set(paneId, ctx) {
      const state = getOrCreate(paneId)
      if (state.ctx?.uri === ctx.uri && state.ctx?.model === ctx.model) return
      state.ctx = ctx
      notify(state, ctx)
    },

    get(paneId) {
      return panes.get(paneId)?.ctx
    },

    subscribe(paneId, cb) {
      const state = getOrCreate(paneId)
      state.listeners.add(cb)
      cb(state.ctx)
      return () => {
        state.listeners.delete(cb)
      }
    },

    clear(paneId) {
      const state = panes.get(paneId)
      if (!state) return
      state.ctx = undefined
      notify(state, undefined)
    },

    clearIfActive(paneId, uri) {
      const state = panes.get(paneId)
      if (!state || state.ctx?.uri !== uri) return
      state.ctx = undefined
      notify(state, undefined)
    },
  }
}

export const activeEditors: ActiveEditorRegistry = createActiveEditorRegistry()
