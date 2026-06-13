/**
 * Per-pane active-editor pub/sub registry.
 *
 * Each pane tracks its current ActiveEditorContext. Subscribers are notified
 * immediately on subscription (with the current context or undefined), and on
 * every subsequent change. `set` is uri-deduped: if the new context carries
 * the same uri as the current one, subscribers are NOT re-notified.
 */

export interface ActiveEditorContext {
  paneId: string
  uri: string // monaco model uri string (athas://editor/...)
  filePath: string
  // opaque handles the controller attaches; satellites may read them:
  model?: unknown // monaco ITextModel
  editor?: unknown // monaco editor instance
}

export interface ActiveEditorRegistry {
  set(paneId: string, ctx: ActiveEditorContext): void
  get(paneId: string): ActiveEditorContext | undefined
  subscribe(paneId: string, cb: (ctx: ActiveEditorContext | undefined) => void): () => void
  clear(paneId: string): void
}

type Listener = (ctx: ActiveEditorContext | undefined) => void

interface PaneState {
  ctx: ActiveEditorContext | undefined
  listeners: Set<Listener>
}

export function createActiveEditorRegistry(): ActiveEditorRegistry {
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
      if (state.ctx?.uri === ctx.uri) return
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
  }
}
