import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { PaneGroup } from '@/features/panes/types/pane'
import type { BrowserContent } from '@/features/panes/types/pane-content'
import { browserApi, type BrowserRect } from '@/lib/browser-bridge'
import { domAgentBody, type DomOp, type PointerOp } from './dom-agent'
import { ensureWebview } from './ensure-webview'
import { webviewCreated } from './webview-registry'

/** How long a wait may take; a wait that runs out is reported, never retried. */
export interface AgentLimits {
  loadMs: number
}

const DEFAULT_LIMITS: AgentLimits = { loadMs: 15_000 }
/** Where a tab nobody has looked at starts; the pane sets the real size when it shows it. */
const PARKED_RECT: BrowserRect = { x: 0, y: 0, width: 1280, height: 800 }
const PAGE_OP_TIMEOUT_MS = 15_000
const EVAL_TIMEOUT_MS = 30_000
const DOM_OPS: ReadonlySet<string> = new Set([
  'snapshot',
  'click',
  'type',
  'press_key',
  'scroll',
  'resolve',
])

export interface TabInfo {
  id: string
  chatId: string
  url: string
  title: string
  loading: boolean
}

export interface AgentRequest {
  chatId: string
  op: string
  tab: string
  args: Record<string, unknown>
}

function pane(chatId: string): PaneGroup & { workspaceId: string } {
  const { panes } = windowPaneStore.getState()
  const found = panes[chatPaneIndex(panes).get(chatId) ?? '']
  if (!found || !found.workspaceId) {
    throw new Error(
      'this chat is not open in the Crowbar window, so it has no pane to hold a browser',
    )
  }
  return { ...found, workspaceId: found.workspaceId }
}

function browserBuffer(id: string): BrowserContent | undefined {
  const buffer = windowPaneStore.getState().bufferActions.getBufferById(id)
  return buffer?.type === 'browser' ? buffer : undefined
}

/** Every browser tab that sits in a chat's pane, tagged with that chat. */
export function listTabs(): TabInfo[] {
  const tabs: TabInfo[] = []
  for (const group of Object.values(windowPaneStore.getState().panes)) {
    if (!group.chatId) continue
    for (const id of group.editorTabIds) {
      const buffer = browserBuffer(id)
      if (!buffer) continue
      tabs.push({
        id,
        chatId: group.chatId,
        url: buffer.url,
        title: buffer.title ?? '',
        loading: buffer.loading ?? false,
      })
    }
  }
  return tabs
}

/** The tab, only if it is in the pane of `chatId`: the check every tab-addressed op starts with. */
function ownedTab(chatId: string, tabId: string): { pane: PaneGroup; buffer: BrowserContent } {
  const group = pane(chatId)
  const buffer = browserBuffer(tabId)
  if (!buffer || !group.editorTabIds.includes(tabId)) {
    throw new Error(`no such tab ${JSON.stringify(tabId)} in this chat`)
  }
  return { pane: group, buffer }
}

function parseUrl(raw: unknown): string {
  const text = typeof raw === 'string' ? raw.trim() : ''
  let url: URL
  try {
    url = new URL(text)
  } catch {
    throw new Error(`${JSON.stringify(text)} is not an absolute URL`)
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:' && text !== 'about:blank') {
    throw new Error(`only http and https pages can be opened, not ${url.protocol}`)
  }
  return url.href
}

function afterMs(ms: number): AbortSignal {
  return AbortSignal.timeout(ms)
}

/** Resolves when the tab's next page load finishes, or when `signal` aborts. */
function nextLoadEnd(id: string, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    let started = false
    const done = () => {
      unsubscribe()
      resolve()
    }
    const unsubscribe = windowPaneStore.subscribe(() => {
      const loading = browserBuffer(id)?.loading ?? false
      if (loading) started = true
      else if (started) done()
    })
    signal.addEventListener('abort', done, { once: true })
  })
}

/**
 * Makes sure the tab's page is running. A tab nobody has looked at gets its
 * webview parked off screen, so no op needs the tab shown or the view changed.
 */
async function liveTab(chatId: string, tabId: string): Promise<void> {
  const { buffer } = ownedTab(chatId, tabId)
  if (buffer.url === '') throw new Error('this tab has no page yet; use browser_navigate first')
  await ensureWebview(tabId, PARKED_RECT, true)
}

/** `liveTab`, and when that makes the webview, its first page load too: an op that
 *  answers before then would hand the agent a blank page. */
async function loadedTab(chatId: string, tabId: string, limits: AgentLimits): Promise<void> {
  const settled =
    webviewCreated(tabId) === undefined ? nextLoadEnd(tabId, afterMs(limits.loadMs)) : undefined
  await liveTab(chatId, tabId)
  await settled
}

async function openTab(chatId: string, args: Record<string, unknown>, limits: AgentLimits) {
  const group = pane(chatId)
  const url = args.url === undefined ? undefined : parseUrl(args.url)
  const existing = group.editorTabIds.find((id) => browserBuffer(id))
  if (existing && args.newTab !== true) {
    if (url) await navigateTab(chatId, existing, url, limits)
    return { tab: existing, reused: true }
  }
  const id = windowPaneStore
    .getState()
    .bufferActions.openContent(
      { type: 'browser', url, workspaceId: group.workspaceId },
      { paneId: group.id },
    )
  if (url) await loadedTab(chatId, id, limits)
  return { tab: id, reused: false }
}

async function navigateTab(chatId: string, tabId: string, url: string, limits: AgentLimits) {
  ownedTab(chatId, tabId)
  // A webview that does not exist yet is made at the new url; only a live one is sent there.
  const wasLive = webviewCreated(tabId) !== undefined
  windowPaneStore.getState().bufferActions.setBrowserLocation(tabId, { url })
  await loadedTab(chatId, tabId, limits)
  if (wasLive) {
    const settled = nextLoadEnd(tabId, afterMs(limits.loadMs))
    await browserApi.navigate(tabId, url)
    await settled
  }
  const now = browserBuffer(tabId)
  return { url: now?.url ?? url, title: now?.title ?? '', loading: now?.loading ?? false }
}

async function runInPage(tabId: string, body: string, timeoutMs: number): Promise<unknown> {
  return JSON.parse(await browserApi.evaluate(tabId, body, timeoutMs))
}

function waitTimeout(args: Record<string, unknown>): number {
  const asked = typeof args.timeoutMs === 'number' ? args.timeoutMs : 10_000
  return Math.min(Math.max(asked, 1), 30_000)
}

/**
 * Carries out one agent request against the browser tabs of `chatId`'s pane.
 * `chatId` comes from the daemon's authenticated session, and every tab it
 * names is re-checked against that pane before anything runs.
 */
export async function runAgentOp(
  { chatId, op, tab, args }: AgentRequest,
  limits: AgentLimits = DEFAULT_LIMITS,
): Promise<unknown> {
  if (op === 'open') return openTab(chatId, args, limits)
  const owned = ownedTab(chatId, tab)

  if (op === 'close') {
    windowPaneStore.getState().paneActions.removeEditorTabFromPane(owned.pane.id, tab)
    return 'closed'
  }
  if (op === 'navigate') return navigateTab(chatId, tab, parseUrl(args.url), limits)

  await liveTab(chatId, tab)
  switch (op) {
    case 'back':
      return runInPage(tab, 'history.back()', PAGE_OP_TIMEOUT_MS).then(() => 'went back')
    case 'forward':
      return runInPage(tab, 'history.forward()', PAGE_OP_TIMEOUT_MS).then(() => 'went forward')
    case 'reload':
      await browserApi.reload(tab)
      return 'reloading'
    case 'eval':
      return runInPage(tab, String(args.js ?? ''), EVAL_TIMEOUT_MS)
    case 'screenshot':
      return browserApi.screenshot(tab)
    case 'wait': {
      const timeoutMs = waitTimeout(args)
      return runInPage(tab, domAgentBody('wait', { ...args, timeoutMs }), timeoutMs + 1_000)
    }
    default:
      if (!DOM_OPS.has(op)) throw new Error(`unknown browser op ${JSON.stringify(op)}`)
      return runInPage(tab, domAgentBody(op as DomOp | PointerOp, args), PAGE_OP_TIMEOUT_MS)
  }
}
