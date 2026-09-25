/**
 * Monaco language-feature providers backed by the daemon's /lsp routes
 * (owner decision 1): completion, hover, signature help, definition,
 * references, rename, code actions, document symbols, code lens, semantic
 * tokens and document formatting. Monaco owns every widget (suggest, hover,
 * parameter hints, peek, rename box, lightbulb, lenses, outline); this module
 * only answers queries (each through `lsp-query.ts`) and runs the server
 * commands lenses and actions carry.
 */
import { editor as monacoEditor, languages, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import type {
  CodeAction,
  CodeLens,
  Command,
  CompletionItem,
  CompletionList,
  DocumentSymbol,
  Hover,
  SignatureHelp,
  SymbolInformation,
  TextEdit,
} from 'vscode-languageserver-types'
import { extensionRegistry } from '@/extensions/registry/extension-registry'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { toast } from '@/features/window/stores/toast-store'
import { fileUri, uriToFsPath, uriToWorkspaceId } from '../lib/editor-uri'
import { langForUri } from '../lib/monaco-adapters'
import { revealInEditor } from '../lib/reveal'
import { LspClient } from './lsp-client'
import { modelTarget, query } from './lsp-query'
import { lspDocumentSemanticTokensProvider } from './semantic-tokens'
import {
  completionToMonaco,
  documentSymbolsToMonaco,
  hoverToMonaco,
  isCommand,
  markerToLspDiagnostic,
  signatureHelpToMonaco,
  toLspPosition,
  toLspRange,
  toMonacoRange,
  workspaceEditByPath,
  type DaemonLocation,
  type DaemonWorkspaceEdit,
} from './lsp-to-monaco'
import { applyWorkspaceEdit } from './workspace-edit'

/** Monaco language ids the daemon's server registry covers. */
const LSP_LANGUAGES = ['go', 'typescript', 'javascript', 'python', 'java', 'c', 'cpp']

/** Monaco command running a server command (and any edit it carries) for a lens or action. */
const RUN_COMMAND = 'crowbar.lsp.runCommand'

function isOutsideWorkspace(path: string): boolean {
  return /^([A-Za-z]:)?[\\/]/.test(path)
}

function modelFor(wsId: string, path: string): Monaco.editor.ITextModel | null {
  return monacoEditor.getModel(Uri.parse(fileUri(wsId, path)))
}

/**
 * Split a workspace edit into Monaco edits for files that have a model (so
 * they land in the editor's undo stack) and the rest, which is applied to the
 * buffers/disk directly.
 */
function splitWorkspaceEdit(
  wsId: string,
  edit: DaemonWorkspaceEdit | null,
): { monacoEdits: Monaco.languages.IWorkspaceTextEdit[]; rest: DaemonWorkspaceEdit | null } {
  const monacoEdits: Monaco.languages.IWorkspaceTextEdit[] = []
  const restChanges: Record<string, TextEdit[]> = {}
  for (const [path, edits] of workspaceEditByPath(edit)) {
    const model = modelFor(wsId, path)
    if (!model) {
      restChanges[path] = edits
      continue
    }
    for (const e of edits) {
      monacoEdits.push({
        resource: model.uri,
        versionId: undefined,
        textEdit: { range: toMonacoRange(e.range), text: e.newText },
      })
    }
  }
  const rest = Object.keys(restChanges).length > 0 ? { changes: restChanges } : null
  return { monacoEdits, rest }
}

/**
 * Apply an edit the server produced: files open in Monaco are edited through
 * their models (one undo step each), the rest through the buffers/disk.
 */
async function applyEdit(wsId: string, edit: DaemonWorkspaceEdit): Promise<void> {
  const rest: Record<string, TextEdit[]> = {}
  for (const [path, edits] of workspaceEditByPath(edit)) {
    const model = modelFor(wsId, path)
    if (!model) {
      rest[path] = edits
      continue
    }
    model.pushStackElement()
    model.pushEditOperations(
      [],
      edits.map((e) => ({ range: toMonacoRange(e.range), text: e.newText })),
      () => null,
    )
    model.pushStackElement()
  }
  if (Object.keys(rest).length > 0) await applyWorkspaceEdit({ changes: rest }, wsId)
}

/**
 * Run a lens's or action's server command: the daemon executes it and hands
 * back the edits the server applied through the editor while it ran.
 */
async function runCommand(
  wsId: string,
  path: string,
  edit: DaemonWorkspaceEdit | null,
  command: Command | null,
): Promise<void> {
  if (edit) await applyEdit(wsId, edit)
  if (!command) return
  const outcome = await LspClient.getInstance().request<{ edits: DaemonWorkspaceEdit[] }>(
    wsId,
    'executeCommand',
    { path, command: command.command, arguments: command.arguments },
  )
  for (const commandEdit of outcome?.edits ?? []) {
    // react-doctor-disable-next-line async-await-in-loop -- FP: ordered on purpose; each workspace/applyEdit's ranges are against the text the previous one left (the daemon collects them in arrival order), so concurrent applies lose edits — pinned by "applies the edits a command made in the order the server made them".
    await applyEdit(wsId, commandEdit)
  }
}

function runCommandFor(
  target: { wsId: string; path: string },
  title: string,
  command: Command | null,
  edit: DaemonWorkspaceEdit | null = null,
): Monaco.languages.Command {
  return { id: RUN_COMMAND, title, arguments: [target.wsId, target.path, edit, command] }
}

/** A lens command the daemon left empty is a label the server cannot run. */
function lensCommand(
  target: { wsId: string; path: string },
  command: Command | undefined,
): Monaco.languages.Command | undefined {
  if (!command) return undefined
  return command.command
    ? runCommandFor(target, command.title, command)
    : { id: '', title: command.title }
}

/**
 * An LSP code action as Monaco's: edits to open files go through `edit` (the
 * undo stack); edits to other files and the server command (the daemon only
 * passes on commands the server runs) go through RUN_COMMAND.
 */
function toCodeAction(
  target: { wsId: string; path: string },
  action: Command | CodeAction,
): Monaco.languages.CodeAction {
  if (isCommand(action)) {
    return { title: action.title, command: runCommandFor(target, action.title, action) }
  }
  const { monacoEdits, rest } = splitWorkspaceEdit(target.wsId, action.edit ?? null)
  const command = action.command ?? null
  return {
    title: action.title,
    kind: action.kind,
    isPreferred: action.isPreferred,
    edit: monacoEdits.length > 0 ? { edits: monacoEdits } : undefined,
    command: rest || command ? runCommandFor(target, action.title, command, rest) : undefined,
  }
}

// Models created only so the references peek can preview files that are not
// open. The ModelRegistry adopts one if the file is later opened; otherwise
// it is disposed when the next peek replaces it.
const peekModels = new Set<Monaco.editor.ITextModel>()

function isHeld(model: Monaco.editor.ITextModel): boolean {
  const uri = model.uri.toString()
  const wsId = uriToWorkspaceId(uri)
  if (wsId && getWorkspaceStore(wsId)?.modelRegistry?.get(uri)) return true
  return monacoEditor.getEditors().some((e) => e.getModel() === model)
}

async function ensurePreviewModels(wsId: string, paths: string[]): Promise<void> {
  for (const model of peekModels) {
    if (!model.isDisposed() && !isHeld(model)) model.dispose()
  }
  peekModels.clear()
  const missing = [...new Set(paths)].filter((p) => !modelFor(wsId, p))
  await Promise.all(
    missing.map(async (path) => {
      const uri = fileUri(wsId, path)
      const open = windowPaneStore
        .getState()
        .buffers.find((b) => b.type === 'editor' && b.path === path && b.workspaceId === wsId)
      const text =
        open && 'content' in open
          ? open.content
          : await readWorkspaceFile(wsId, path).catch(() => null)
      if (text === null || modelFor(wsId, path)) return
      peekModels.add(monacoEditor.createModel(text, langForUri(uri), Uri.parse(uri)))
    }),
  )
}

function toMonacoLocations(wsId: string, locations: DaemonLocation[]): Monaco.languages.Location[] {
  const inside: Monaco.languages.Location[] = []
  for (const loc of locations) {
    if (isOutsideWorkspace(loc.filePath)) continue
    inside.push({ uri: Uri.parse(fileUri(wsId, loc.filePath)), range: toMonacoRange(loc.range) })
  }
  return inside
}

function recordJumpFrom(source: Monaco.editor.ICodeEditor): void {
  const model = source.getModel()
  const target = model ? modelTarget(model) : null
  const position = source.getPosition()
  if (!model || !target || !position) return
  const buffer = windowPaneStore
    .getState()
    .buffers.find(
      (b) => b.type === 'editor' && b.path === target.path && b.workspaceId === target.wsId,
    )
  if (!buffer) return
  useJumpListStore.getState().actions.pushEntry({
    bufferId: buffer.id,
    workspaceId: target.wsId,
    filePath: target.path,
    line: position.lineNumber - 1,
    column: position.column - 1,
    offset: model.getOffsetAt(position),
    scrollTop: source.getScrollTop(),
    scrollLeft: source.getScrollLeft(),
  })
}

function positionOf(selectionOrPosition?: Monaco.IRange | Monaco.IPosition) {
  if (!selectionOrPosition) return undefined
  if ('lineNumber' in selectionOrPosition) return toLspPosition(selectionOrPosition)
  return {
    line: selectionOrPosition.startLineNumber - 1,
    character: selectionOrPosition.startColumn - 1,
  }
}

function snippetSuggestions(
  model: Monaco.editor.ITextModel,
  position: Monaco.IPosition,
): Monaco.languages.CompletionList {
  const target = modelTarget(model)
  const languageId = target ? extensionRegistry.getLanguageId(target.path) : null
  if (!languageId) return { suggestions: [] }
  const word = model.getWordUntilPosition(position)
  const range = {
    startLineNumber: position.lineNumber,
    startColumn: word.startColumn,
    endLineNumber: position.lineNumber,
    endColumn: word.endColumn,
  }
  const suggestions = extensionRegistry
    .getSnippetsForLanguage(languageId)
    .map((snippet): Monaco.languages.CompletionItem => ({
      label: snippet.prefix,
      kind: languages.CompletionItemKind.Snippet,
      detail: snippet.description || 'Snippet',
      insertText: Array.isArray(snippet.body) ? snippet.body.join('\n') : snippet.body,
      insertTextRules: languages.CompletionItemInsertTextRule.InsertAsSnippet,
      range,
    }))
  return { suggestions }
}

let registered = false

/** Register the providers once per page (idempotent). */
export function registerLspProviders(): void {
  if (registered) return
  registered = true
  const selector = LSP_LANGUAGES

  // Page-lifetime registrations: Monaco is a page singleton and so are these.
  monacoEditor.registerCommand(
    RUN_COMMAND,
    (
      _accessor: unknown,
      wsId: string,
      path: string,
      edit: DaemonWorkspaceEdit | null,
      command: Command | null,
    ) => {
      void runCommand(wsId, path, edit, command).catch((error: unknown) =>
        toast.error('Command failed', error instanceof Error ? error.message : undefined),
      )
    },
  )

  // Cross-file navigation (go to definition, peek "open") lands in a tab.
  monacoEditor.registerEditorOpener({
    openCodeEditor(source, resource, selectionOrPosition) {
      const wsId = uriToWorkspaceId(resource.toString())
      if (!wsId) return false
      recordJumpFrom(source)
      void revealInEditor({
        workspaceId: wsId,
        path: uriToFsPath(resource.toString()),
        position: positionOf(selectionOrPosition),
      }).catch((error: unknown) =>
        toast.error('Could not open file', error instanceof Error ? error.message : undefined),
      )
      return true
    },
  })

  languages.registerCompletionItemProvider('*', {
    provideCompletionItems: (model, position) => snippetSuggestions(model, position),
  })

  languages.registerCompletionItemProvider(selector, {
    triggerCharacters: ['.', ':', '<', '"', "'", '/', '@', '>', '#'],
    async provideCompletionItems(model, position, _context, token) {
      const response = await query<CompletionList | CompletionItem[]>(
        model,
        'completion',
        { position: toLspPosition(position) },
        token,
      )
      const word = model.getWordUntilPosition(position)
      const defaultRange = {
        startLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endLineNumber: position.lineNumber,
        endColumn: word.endColumn,
      }
      return completionToMonaco(response?.result ?? null, defaultRange)
    },
  })

  languages.registerHoverProvider(selector, {
    async provideHover(model, position, token) {
      const response = await query<Hover>(
        model,
        'hover',
        { position: toLspPosition(position) },
        token,
      )
      return hoverToMonaco(response?.result ?? null)
    },
  })

  languages.registerSignatureHelpProvider(selector, {
    signatureHelpTriggerCharacters: ['(', ','],
    signatureHelpRetriggerCharacters: [')'],
    async provideSignatureHelp(model, position, token) {
      const response = await query<SignatureHelp>(
        model,
        'signatureHelp',
        { position: toLspPosition(position) },
        token,
      )
      return signatureHelpToMonaco(response?.result ?? null)
    },
  })

  languages.registerDefinitionProvider(selector, {
    async provideDefinition(model, position, token) {
      const response = await query<DaemonLocation[]>(
        model,
        'definition',
        { position: toLspPosition(position) },
        token,
        'Could not go to definition',
      )
      const locations = response?.result ?? []
      if (!response) return null
      if (locations.length > 0 && locations.every((l) => isOutsideWorkspace(l.filePath))) {
        toast.error('Definition is outside this workspace', locations[0]?.filePath)
        return null
      }
      return toMonacoLocations(response.target.wsId, locations)
    },
  })

  languages.registerReferenceProvider(selector, {
    async provideReferences(model, position, _context, token) {
      const response = await query<DaemonLocation[]>(
        model,
        'references',
        { position: toLspPosition(position) },
        token,
        'Could not find references',
      )
      if (!response?.result) return null
      const locations = toMonacoLocations(response.target.wsId, response.result)
      await ensurePreviewModels(
        response.target.wsId,
        locations.map((l) => uriToFsPath(l.uri.toString())),
      )
      return locations
    },
  })

  languages.registerRenameProvider(selector, {
    async provideRenameEdits(model, position, newName, token) {
      const response = await query<DaemonWorkspaceEdit>(
        model,
        'rename',
        { position: toLspPosition(position), newName },
        token,
        'Rename failed',
      )
      if (!response?.result) return { edits: [] }
      const { monacoEdits, rest } = splitWorkspaceEdit(response.target.wsId, response.result)
      if (rest) await applyWorkspaceEdit(rest, response.target.wsId)
      return { edits: monacoEdits }
    },
  })

  languages.registerCodeActionProvider(selector, {
    async provideCodeActions(model, range, context, token) {
      const markers = context.markers
      // The lightbulb asks on every cursor move; only go to the daemon when
      // there is something to fix here or the user asked explicitly.
      if (context.trigger !== languages.CodeActionTriggerType.Invoke && markers.length === 0) {
        return { actions: [], dispose() {} }
      }
      const response = await query<Array<Command | CodeAction>>(
        model,
        'codeAction',
        { range: toLspRange(range), diagnostics: markers.map(markerToLspDiagnostic) },
        token,
      )
      if (!response) return { actions: [], dispose() {} }
      const actions = (response.result ?? []).map((action) => toCodeAction(response.target, action))
      return { actions, dispose() {} }
    },
  })

  languages.registerDocumentSymbolProvider(selector, {
    async provideDocumentSymbols(model, token) {
      const response = await query<Array<DocumentSymbol | SymbolInformation>>(
        model,
        'documentSymbol',
        {},
        token,
      )
      return documentSymbolsToMonaco(response?.result ?? null)
    },
  })

  languages.registerCodeLensProvider(selector, {
    async provideCodeLenses(model, token) {
      const response = await query<CodeLens[]>(model, 'codeLens', {}, token)
      const target = response?.target
      const lenses = target
        ? (response.result ?? []).map((lens) => ({
            range: toMonacoRange(lens.range),
            command: lensCommand(target, lens.command),
            raw: lens,
          }))
        : []
      return { lenses, dispose() {} }
    },
    async resolveCodeLens(model, lens, token) {
      const raw = (lens as Monaco.languages.CodeLens & { raw?: CodeLens }).raw
      if (lens.command || !raw) return lens
      const response = await query<CodeLens>(model, 'codeLensResolve', { lens: raw }, token)
      const command = response ? lensCommand(response.target, response.result?.command) : undefined
      return command ? { ...lens, command } : lens
    },
  })

  languages.registerDocumentSemanticTokensProvider(selector, lspDocumentSemanticTokensProvider)

  languages.registerDocumentFormattingEditProvider(selector, {
    async provideDocumentFormattingEdits(model, options, token) {
      const response = await query<TextEdit[]>(
        model,
        'formatting',
        { options: { tabSize: options.tabSize, insertSpaces: options.insertSpaces } },
        token,
        'Formatting failed',
      )
      return (response?.result ?? []).map((e) => ({
        range: toMonacoRange(e.range),
        text: e.newText,
      }))
    },
  })
}
