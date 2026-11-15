import { afterEach, describe, expect, it, vi } from 'vitest'
import { shellConfig } from '@/sdk/shell-config'

const win = window as unknown as Record<string, unknown>

afterEach(() => {
  delete win.crowbar
  delete win.__CROWBAR__
})

describe('shellConfig', () => {
  it.each([
    [
      'is null when the page is not hosted by the shell',
      false,
      { api: 'crowbar://local', host: 'local' },
      null,
    ],
    ['is null when the shell injected no endpoints', true, undefined, null],
    [
      'returns the endpoints, with no relay when none was issued',
      true,
      { api: 'crowbar://local', host: 'local' },
      { api: 'crowbar://local', host: 'local', ws: null },
    ],
  ])('%s', (_name, hosted, injected, expected) => {
    if (hosted) win.crowbar = { post: vi.fn(), onmessage: null }
    win.__CROWBAR__ = injected
    expect(shellConfig()).toEqual(expected)
  })
})
