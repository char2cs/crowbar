/**
 * Code lenses and code actions carry server commands; the providers turn
 * them into one Monaco command that has the daemon execute them and applies
 * the edits the server made while they ran.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type * as Monaco from 'monaco-editor'

const monaco = vi.hoisted(() => ({
  commands: new Map<string, (...args: unknown[]) => void>(),
  providers: new Map<string, Record<string, (...args: never[]) => unknown>>(),
  models: new Map<string, { pushEditOperations: ReturnType<typeof vi.fn> }>(),
}))

vi.mock('monaco-editor/esm/vs/editor/editor.api.js', () => {
  const register = (kind: string) => (_selector: unknown, provider: never) => {
    monaco.providers.set(kind, provider)
    return { dispose() {} }
  }
  return {
    Uri: { parse: (value: string) => ({ toString: () => value }) },
    editor: {
      registerCommand: (id: string, handler: (...args: unknown[]) => void) => {
        monaco.commands.set(id, handler)
      },
      registerEditorOpener: () => ({ dispose() {} }),
      getModel: (uri: { toString(): string }) => {
        const model = monaco.models.get(uri.toString())
        return model ? { ...model, uri, pushStackElement() {} } : null
      },
      getEditors: () => [],
    },
    languages: {
      // language-contributions (reached through monaco-adapters) registers at import.
      getLanguages: () => [],
      register: () => {},
      setMonarchTokensProvider: () => {},
      registerDocumentRangeSemanticTokensProvider: () => {},
      CodeActionTriggerType: { Invoke: 1, Auto: 2 },
      registerCompletionItemProvider: register('completion'),
      registerHoverProvider: register('hover'),
      registerSignatureHelpProvider: register('signatureHelp'),
      registerDefinitionProvider: register('definition'),
      registerReferenceProvider: register('references'),
      registerRenameProvider: register('rename'),
      registerCodeActionProvider: register('codeAction'),
      registerDocumentSymbolProvider: register('documentSymbol'),
      registerCodeLensProvider: register('codeLens'),
      registerDocumentSemanticTokensProvider: register('semanticTokens'),
      registerDocumentFormattingEditProvider: register('formatting'),
    },
  }
})
vi.mock('@/features/editor/lsp/lsp-query', () => ({ query: vi.fn(), modelTarget: vi.fn() }))
vi.mock('@/features/editor/lsp/lsp-client', () => {
  const request = vi.fn()
  return { LspClient: { getInstance: () => ({ request }) } }
})

import { fileUri } from '@/features/editor/lib/editor-uri'
import { LspClient } from '@/features/editor/lsp/lsp-client'
import { query } from '@/features/editor/lsp/lsp-query'
import { registerLspProviders } from '@/features/editor/lsp/monaco-lsp-providers'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

registerLspProviders()

const target = { wsId: 'ws1', path: 'main.go' }
const model = {} as Monaco.editor.ITextModel
const none = {} as Monaco.CancellationToken
const request = vi.mocked(LspClient.getInstance().request)
const range = { start: { line: 0, character: 0 }, end: { line: 0, character: 0 } }

function provider(kind: string) {
  const found = monaco.providers.get(kind)
  if (!found) throw new Error(`${kind} provider not registered`)
  return found as Record<string, (...args: unknown[]) => Promise<unknown>>
}

async function run(command: Monaco.languages.Command | undefined) {
  const handler = monaco.commands.get(command?.id ?? '')
  if (!handler) throw new Error(`no command ${command?.id}`)
  handler(undefined, ...(command?.arguments ?? []))
  await vi.waitFor(() => expect(request).toHaveBeenCalled())
}

describe('LSP commands', () => {
  beforeEach(() => {
    vi.mocked(query).mockReset()
    request.mockReset()
    monaco.models.clear()
  })

  it('makes a server lens runnable and keeps a client-only lens a label', async () => {
    vi.mocked(query).mockResolvedValue({
      target,
      result: [
        { range, command: { title: 'run test', command: 'gopls.test', arguments: [1] } },
        { range, command: { title: '3 references', command: '' } },
      ],
    })

    const { lenses } = (await provider('codeLens').provideCodeLenses(model, none)) as {
      lenses: Monaco.languages.CodeLens[]
    }

    expect(lenses[0].command).toMatchObject({ title: 'run test', id: 'crowbar.lsp.runCommand' })
    expect(lenses[1].command).toEqual({ id: '', title: '3 references' })

    const goMod = { pushEditOperations: vi.fn() }
    monaco.models.set(fileUri('ws1', 'go.mod'), goMod)
    request.mockResolvedValue({
      result: null,
      edits: [{ changes: { 'go.mod': [{ range, newText: 'require x\n' }] } }],
    })
    await run(lenses[0].command)

    expect(request).toHaveBeenCalledWith('ws1', 'executeCommand', {
      path: 'main.go',
      command: 'gopls.test',
      arguments: [1],
    })
    await vi.waitFor(() =>
      expect(goMod.pushEditOperations).toHaveBeenCalledWith(
        [],
        [expect.objectContaining({ text: 'require x\n' })],
        expect.any(Function),
      ),
    )
  })

  it('runs a command-only code action through the daemon', async () => {
    vi.mocked(query).mockResolvedValue({
      target,
      result: [{ title: 'Tidy', command: 'gopls.tidy', arguments: [] }],
    })

    const { actions } = (await provider('codeAction').provideCodeActions(
      model,
      {},
      { markers: [], trigger: 1 },
      none,
    )) as { actions: Monaco.languages.CodeAction[] }

    expect(actions).toHaveLength(1)
    request.mockResolvedValue({ result: null, edits: [] })
    await run(actions[0].command)
    expect(request).toHaveBeenCalledWith('ws1', 'executeCommand', {
      path: 'main.go',
      command: 'gopls.tidy',
      arguments: [],
    })
  })

  it('applies the edits a command made in the order the server made them', async () => {
    resetWindowPaneStoreForTests()
    vi.mocked(query).mockResolvedValue({
      target,
      result: [{ title: 'Fill', command: 'gopls.fill', arguments: [] }],
    })
    const { actions } = (await provider('codeAction').provideCodeActions(
      model,
      {},
      { markers: [], trigger: 1 },
      none,
    )) as { actions: Monaco.languages.CodeAction[] }
    // An open buffer with no Monaco model: its edits go read → apply → write.
    const id = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: 'fill.go',
      name: 'fill.go',
      content: 'a',
      workspaceId: 'ws1',
    })
    const at = (character: number) => ({
      start: { line: 0, character },
      end: { line: 0, character },
    })
    // The second edit's range is against the text the first one left.
    request.mockResolvedValue({
      result: null,
      edits: [
        { changes: { 'fill.go': [{ range: at(0), newText: 'x' }] } },
        { changes: { 'fill.go': [{ range: at(1), newText: 'y' }] } },
      ],
    })

    await run(actions[0].command)

    const content = () => {
      const buffer = windowPaneStore.getState().buffers.find((b) => b.id === id)
      return buffer && 'content' in buffer ? buffer.content : undefined
    }
    await vi.waitFor(() => expect(content()).toBe('xya'))
  })
})
