// Per-pane active-editor pub/sub. Subscribers get the current context at once, then every change.
// `set` dedups on uri AND model identity: a reopened file keeps its uri but gets a new model, and
// keeping the disposed one would crash satellite reads with 'Model is disposed!'.

export interface ActiveEditorContext {
  paneId: string
  uri: string // monaco model uri string (crowbar://editor/...)
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
  /** Clear the pane's context only if it still points at `uri` (buffer close drops the disposed
   *  model so a reopen doesn't keep it); a no-op once the pane moved to another uri. */
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
      // Dedup on uri AND model identity: a reopened file reuses the uri but gets
      // a FRESH model (the prior one was disposed on close), so deduping on uri
      // alone would retain the disposed model and never re-notify satellites.
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

/** The window's one registry: every pane publishes here, whatever workspace it shows. */
export const activeEditors: ActiveEditorRegistry = createActiveEditorRegistry()
