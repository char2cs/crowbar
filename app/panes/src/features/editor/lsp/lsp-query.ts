// One cancellable daemon LSP request for a Monaco model, sent to the model's own workspace (a host
// document has no server). It flushes the pending didChange first, so the server answers against
// the text on screen, and aborts when Monaco cancels.
import type * as Monaco from 'monaco-editor'
import { refFromUri } from '../documents/document-ref'
import { LspClient } from './lsp-client'
import { report } from '@/lib/errors/report'

export interface ModelTarget {
  wsId: string
  path: string
}

function modelTarget(model: Monaco.editor.ITextModel): ModelTarget | null {
  const ref = refFromUri(model.uri.toString())
  return ref?.authority === 'workspace' ? { wsId: ref.workspaceId, path: ref.path } : null
}

function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

/** Background queries (hover, completion, lenses, tokens) degrade to null on failure; user-invoked
 *  ones pass `surface` to be told. */
export async function query<T>(
  model: Monaco.editor.ITextModel,
  route: string,
  body: Record<string, unknown>,
  token: Monaco.CancellationToken,
  surface?: string,
): Promise<{ target: ModelTarget; result: T | null } | null> {
  const target = modelTarget(model)
  if (!target || token.isCancellationRequested) return null
  const client = LspClient.getInstance()
  const controller = new AbortController()
  const cancel = token.onCancellationRequested(() => controller.abort())
  try {
    await client.flushChange(target.wsId, target.path)
    if (token.isCancellationRequested) return null
    const result = await client.request<T>(
      target.wsId,
      route,
      { path: target.path, ...body },
      controller.signal,
    )
    return { target, result }
  } catch (error) {
    if (!isAbort(error) && surface) {
      report(error, { action: surface })
    }
    return null
  } finally {
    cancel.dispose()
  }
}
