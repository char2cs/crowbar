import { afterEach, describe, expect, it, vi } from 'vitest'
import { canAskShellMenu, popShellMenu } from '@/sdk/shell-menu'

const win = window as unknown as Record<string, unknown>

function hostedOn(api: string) {
  win.crowbar = { post: vi.fn(), onmessage: null }
  win.__CROWBAR__ = { api, host: 'paired-1' }
}

afterEach(() => {
  delete win.crowbar
  delete win.__CROWBAR__
  vi.restoreAllMocks()
})

describe('popShellMenu', () => {
  it('asks the shell of the page’s own host and resolves to the chosen path', async () => {
    hostedOn('crowbar://paired-1')
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(JSON.stringify({ path: [0] })))
    const entries = [{ kind: 'separator' as const }]

    await expect(popShellMenu(entries, { x: 1, y: 2 })).resolves.toEqual([0])
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('crowbar://paired-1/__crowbar/menu')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body as string)).toEqual({ entries, x: 1, y: 2 })
  })

  it.each([
    ['the shell refuses the page', true, '403'],
    ['the shell does not host the page', false, 'not hosted'],
  ])('rejects when %s', async (_name, hosted, reason) => {
    if (hosted) hostedOn('crowbar://local')
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}', { status: 403 }))
    expect(canAskShellMenu()).toBe(hosted)
    await expect(popShellMenu([], { x: 0, y: 0 })).rejects.toThrow(reason)
  })
})
