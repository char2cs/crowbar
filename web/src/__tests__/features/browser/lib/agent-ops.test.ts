import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listTabs, runAgentOp, type AgentLimits } from '@/features/browser/lib/agent-ops'
import { registerWebview, setWebviewVisible } from '@/features/browser/lib/webview-registry'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

const api = vi.hoisted(() => ({
  navigate: vi.fn().mockResolvedValue(undefined),
  reload: vi.fn().mockResolvedValue(undefined),
  close: vi.fn().mockResolvedValue(undefined),
  evaluate: vi.fn(),
  screenshot: vi.fn(),
}))
vi.mock('@/lib/browser-bridge', () => ({ browserApi: api }))

const QUICK: AgentLimits = { liveMs: 20, showMs: 20, loadMs: 20 }
const state = () => windowPaneStore.getState()

/** Stands in for BrowserPane: a tab with a URL gets a live webview, shown or not. */
function simulatePane(visible = true) {
  const mounted = new Set<string>()
  const mount = () => {
    for (const buffer of state().buffers) {
      if (buffer.type !== 'browser' || buffer.url === '' || mounted.has(buffer.id)) continue
      mounted.add(buffer.id)
      registerWebview(buffer.id, Promise.resolve())
      setWebviewVisible(buffer.id, visible)
    }
  }
  mount()
  return windowPaneStore.subscribe(mount)
}

function paneOf(chatId: string) {
  return state()
    .paneActions.getAllPaneGroups()
    .find((p) => p.chatId === chatId)
}

const req = (chatId: string, op: string, tab = '', args: Record<string, unknown> = {}) => ({
  chatId,
  op,
  tab,
  args,
})

let stopPane: () => void

beforeEach(() => {
  vi.clearAllMocks()
  resetWindowPaneStoreForTests()
  state().paneActions.setActiveProject('p1')
  state().paneActions.openChat('chat-a', { workspaceId: 'ws-a' })
  state().paneActions.openChat('chat-b', { workspaceId: 'ws-b' })
  stopPane = simulatePane()
})

afterEach(() => stopPane())

async function openFor(chatId: string, url = 'https://a.test/') {
  const out = (await runAgentOp(req(chatId, 'open', '', { url }))) as { tab: string }
  return out.tab
}

describe('open', () => {
  it('opens a tab in the chat’s own pane and workspace, nowhere else', async () => {
    const tab = await openFor('chat-a')
    expect(paneOf('chat-a')?.editorTabIds).toEqual([tab])
    expect(paneOf('chat-b')?.editorTabIds).toEqual([])
    expect(state().bufferActions.getBufferById(tab)).toMatchObject({
      type: 'browser',
      url: 'https://a.test/',
      workspaceId: 'ws-a',
    })
    expect(paneOf('chat-a')?.activeEditorTabId).toBe(tab)
  })

  it('reuses the chat’s browser tab unless a new one is asked for', async () => {
    const first = await openFor('chat-a')
    expect(await runAgentOp(req('chat-a', 'open'))).toEqual({ tab: first, reused: true })
    const second = (await runAgentOp(req('chat-a', 'open', '', { newTab: true }))) as {
      tab: string
    }
    expect(second.tab).not.toBe(first)
    expect(listTabs().filter((t) => t.chatId === 'chat-a')).toHaveLength(2)
  })

  it('opens a blank tab when no url is given', async () => {
    const out = (await runAgentOp(req('chat-a', 'open'))) as { tab: string }
    expect(state().bufferActions.getBufferById(out.tab)).toMatchObject({ url: '' })
  })

  it('refuses a chat that is not open in the window', async () => {
    await expect(runAgentOp(req('chat-ghost', 'open'))).rejects.toThrow(/not open in the Crowbar/)
  })

  it('refuses anything but http and https pages', async () => {
    for (const bad of ['file:///etc/passwd', 'javascript:alert(1)', 'crowbar://x', 'nope']) {
      await expect(runAgentOp(req('chat-a', 'open', '', { url: bad }))).rejects.toThrow()
    }
    expect(listTabs()).toEqual([])
  })
})

describe('scope', () => {
  it('lists tabs with the chat that owns them', async () => {
    const a = await openFor('chat-a')
    const b = await openFor('chat-b', 'https://b.test/')
    expect(listTabs().map((t) => [t.id, t.chatId])).toEqual(
      expect.arrayContaining([
        [a, 'chat-a'],
        [b, 'chat-b'],
      ]),
    )
  })

  it('never lets one chat act on a tab in another chat’s pane', async () => {
    const theirs = await openFor('chat-b', 'https://b.test/')
    for (const op of ['navigate', 'eval', 'snapshot', 'screenshot', 'close', 'reload']) {
      await expect(
        runAgentOp(req('chat-a', op, theirs, { url: 'https://evil.test/', js: '1' })),
      ).rejects.toThrow(/no such tab/)
    }
    await expect(runAgentOp(req('chat-a', 'eval', 'guessed', { js: '1' }))).rejects.toThrow(
      /no such tab/,
    )
    expect(api.evaluate).not.toHaveBeenCalled()
    expect(api.navigate).not.toHaveBeenCalled()
    expect(api.screenshot).not.toHaveBeenCalled()
    expect(listTabs().find((t) => t.id === theirs)).toBeDefined()
  })
})

describe('page ops', () => {
  it('runs a snapshot as page script and returns its value', async () => {
    const tab = await openFor('chat-a')
    api.evaluate.mockResolvedValue(JSON.stringify('Page: x'))
    expect(await runAgentOp(req('chat-a', 'snapshot', tab))).toBe('Page: x')
    const [id, body] = api.evaluate.mock.calls[0]
    expect(id).toBe(tab)
    expect(body).toContain('run("snapshot", {})')
  })

  it('runs eval code as given and returns the parsed result', async () => {
    const tab = await openFor('chat-a')
    api.evaluate.mockResolvedValue('{"n":2}')
    expect(await runAgentOp(req('chat-a', 'eval', tab, { js: 'return {n: 2}' }))).toEqual({ n: 2 })
    expect(api.evaluate.mock.calls[0][1]).toBe('return {n: 2}')
  })

  it('bounds how long a wait may run', async () => {
    const tab = await openFor('chat-a')
    api.evaluate.mockResolvedValue('"ok"')
    await runAgentOp(req('chat-a', 'wait', tab, { condition: 'load', timeoutMs: 9_999_999 }))
    const [, body, timeout] = api.evaluate.mock.calls[0]
    expect(body).toContain('"timeoutMs":30000')
    expect(timeout).toBe(31_000)
  })

  it('takes a screenshot through the native path', async () => {
    const tab = await openFor('chat-a')
    api.screenshot.mockResolvedValue({ png: 'AAAA', width: 10, height: 5 })
    expect(await runAgentOp(req('chat-a', 'screenshot', tab))).toEqual({
      png: 'AAAA',
      width: 10,
      height: 5,
    })
  })

  it('refuses page ops on a tab with no page yet', async () => {
    const out = (await runAgentOp(req('chat-a', 'open'))) as { tab: string }
    await expect(runAgentOp(req('chat-a', 'snapshot', out.tab))).rejects.toThrow(/browser_navigate/)
  })

  it('rejects an unknown op', async () => {
    const tab = await openFor('chat-a')
    await expect(runAgentOp(req('chat-a', 'teleport', tab))).rejects.toThrow(/unknown browser op/)
  })
})

describe('navigation', () => {
  it('navigates a live tab natively and records where it went', async () => {
    const tab = await openFor('chat-a')
    const out = await runAgentOp(
      req('chat-a', 'navigate', tab, { url: 'https://next.test/' }),
      QUICK,
    )
    expect(api.navigate).toHaveBeenCalledWith(tab, 'https://next.test/')
    expect(out).toMatchObject({ url: 'https://next.test/' })
  })

  it('gives a blank tab its page by setting the url the pane then loads', async () => {
    const out = (await runAgentOp(req('chat-a', 'open'))) as { tab: string }
    await runAgentOp(req('chat-a', 'navigate', out.tab, { url: 'https://first.test/' }))
    expect(api.navigate).not.toHaveBeenCalled()
    expect(state().bufferActions.getBufferById(out.tab)).toMatchObject({
      url: 'https://first.test/',
    })
  })

  it('waits for the page load to finish when the webview reports one', async () => {
    const tab = await openFor('chat-a')
    api.navigate.mockImplementation(async () => {
      state().bufferActions.setBrowserLocation(tab, { loading: true })
      state().bufferActions.setBrowserLocation(tab, { loading: false, title: 'Next' })
    })
    const out = await runAgentOp(req('chat-a', 'navigate', tab, { url: 'https://next.test/' }))
    expect(out).toMatchObject({ title: 'Next', loading: false })
  })
})

describe('a tab that is not on screen', () => {
  it('is brought to the front of its pane before the page is touched', async () => {
    const first = await openFor('chat-a')
    const second = (await runAgentOp(
      req('chat-a', 'open', '', { newTab: true, url: 'https://c.test/' }),
    )) as { tab: string }
    expect(paneOf('chat-a')?.activeEditorTabId).toBe(second.tab)
    api.evaluate.mockResolvedValue('1')
    await runAgentOp(req('chat-a', 'eval', first, { js: 'return 1' }))
    expect(paneOf('chat-a')?.activeEditorTabId).toBe(first)
  })

  it('reports that it cannot be shown instead of hanging', async () => {
    stopPane()
    stopPane = simulatePane(false)
    const out = (await runAgentOp(req('chat-a', 'open'))) as { tab: string }
    await expect(
      runAgentOp(req('chat-a', 'navigate', out.tab, { url: 'https://hidden.test/' }), QUICK),
    ).rejects.toThrow(/cannot be shown/)
    await expect(runAgentOp(req('chat-a', 'snapshot', out.tab), QUICK)).rejects.toThrow(
      /cannot be shown/,
    )
    expect(api.evaluate).not.toHaveBeenCalled()
  })
})

describe('close', () => {
  it('removes the tab from its pane and frees its webview', async () => {
    const tab = await openFor('chat-a')
    expect(await runAgentOp(req('chat-a', 'close', tab))).toBe('closed')
    expect(listTabs()).toEqual([])
    expect(api.close).toHaveBeenCalledWith(tab)
  })
})
