import { afterEach, describe, expect, it, vi } from 'vitest'

const host = vi.hoisted(() => ({
  tauri: true,
  mobile: true,
  invoke: vi.fn(async (): Promise<unknown> => undefined),
  unregister: vi.fn(async () => undefined),
  addPluginListener: vi.fn(),
}))
vi.mock('@tauri-apps/api/core', () => ({
  invoke: host.invoke,
  addPluginListener: host.addPluginListener,
}))
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: () => host.tauri }))
vi.mock('@/utils/platform', () => ({
  get IS_MOBILE() {
    return host.mobile
  },
}))

import {
  hasNativeChrome,
  nativeConfirm,
  nativePrompt,
  onNativeChromeIntent,
  setNativeChrome,
  setNativeModelPicker,
  setNativeSidebar,
  type NativeChrome,
} from '@/lib/native-chrome'

const model: NativeChrome = {
  accent: '#606c38',
  welcome: { visible: true, label: 'Let’s build together' },
  brandGround: 'color(display-p3 0.15686 0.21176 0.09412)',
  connect: {
    open: false,
    stage: 'address',
    name: '',
    address: '',
    code: '',
    fingerprint: '',
    busy: false,
    error: null,
  },
  headers: { chat: [], ide: [] },
  reveal: 'none',
  ack: 0,
  settings: { open: false },
  appearance: 'system',
  palette: { surface: '#fcfcfa', raised: '#262624', text: '#1b1b19', secondaryText: '#666666' },
}

afterEach(() => {
  host.tauri = true
  host.mobile = true
  host.invoke.mockClear()
  host.unregister.mockClear()
  host.addPluginListener.mockReset()
})

describe('setNativeChrome', () => {
  it('hands the whole view-model to the phone’s chrome plugin', () => {
    setNativeChrome(model)
    expect(host.invoke).toHaveBeenCalledWith('plugin:crowbar-chrome|set_chrome', model)
  })

  it.each([
    ['outside Tauri', { tauri: false, mobile: true }],
    ['in the desktop app', { tauri: true, mobile: false }],
  ])('sends nothing %s', (_where, where) => {
    Object.assign(host, where)
    setNativeChrome(model)
    expect(host.invoke).not.toHaveBeenCalled()
    expect(hasNativeChrome()).toBe(false)
  })
})

describe('setNativeSidebar', () => {
  it('hands the whole chat list to the plugin, and nothing without the phone’s chrome', () => {
    const sidebar = {
      focusedChatId: null,
      hosts: [],
      activeProjectId: null,
      projects: [],
      rows: [],
      active: [],
      collapsed: [],
      actions: {},
      drop: null,
      readout: null,
    }
    setNativeSidebar(sidebar)
    expect(host.invoke).toHaveBeenCalledWith('plugin:crowbar-chrome|set_sidebar', sidebar)

    host.invoke.mockClear()
    host.mobile = false
    setNativeSidebar(sidebar)
    expect(host.invoke).not.toHaveBeenCalled()
  })
})

describe('setNativeModelPicker', () => {
  it('hands the whole picker to the plugin', () => {
    const picker = {
      open: true,
      providers: [],
      selected: null,
      efforts: ['low', 'high'],
      effort: 'low',
    }
    setNativeModelPicker(picker)
    expect(host.invoke).toHaveBeenCalledWith('plugin:crowbar-chrome|set_model_picker', picker)
  })
})

// Modal questions the main webview asks over whatever native sheet is up.
describe('nativeConfirm and nativePrompt', () => {
  const confirm = {
    title: 'Revoke host?',
    message: 'It must be paired again.',
    confirmLabel: 'Revoke',
    cancelLabel: 'Cancel',
    destructive: true,
  }
  const prompt = {
    title: 'Rename chat',
    message: null,
    value: 'Untitled',
    placeholder: 'Name',
    confirmLabel: 'Rename',
  }

  it('asks the plugin and answers with the user’s choice', async () => {
    host.invoke.mockResolvedValueOnce({ confirmed: true })
    await expect(nativeConfirm(confirm)).resolves.toBe(true)
    expect(host.invoke).toHaveBeenCalledWith('plugin:crowbar-chrome|confirm', confirm)

    host.invoke.mockResolvedValueOnce({ value: 'Renamed' })
    await expect(nativePrompt(prompt)).resolves.toBe('Renamed')
    expect(host.invoke).toHaveBeenCalledWith('plugin:crowbar-chrome|prompt', prompt)
  })

  it('answers no without the phone’s chrome', async () => {
    host.mobile = false
    await expect(nativeConfirm(confirm)).resolves.toBe(false)
    await expect(nativePrompt(prompt)).resolves.toBeNull()
    expect(host.invoke).not.toHaveBeenCalled()
  })
})

describe('onNativeChromeIntent', () => {
  it('delivers the plugin’s intents until unsubscribed', async () => {
    let deliver!: (intent: unknown) => void
    host.addPluginListener.mockImplementation(async (_plugin, _event, cb) => {
      deliver = cb
      return { unregister: host.unregister }
    })
    const seen = vi.fn()

    const stop = onNativeChromeIntent(seen)
    await vi.waitFor(() => expect(deliver).toBeDefined())
    deliver({ type: 'start' })
    stop()

    expect(host.addPluginListener).toHaveBeenCalledWith('crowbar-chrome', 'intent', seen)
    expect(seen).toHaveBeenCalledWith({ type: 'start' })
    await vi.waitFor(() => expect(host.unregister).toHaveBeenCalledOnce())
  })

  // Unsubscribed before the plugin answered: the listener it hands back is let go at once.
  it('lets go of a listener that registers after the unsubscribe', async () => {
    let registered!: (listener: { unregister: () => Promise<void> }) => void
    host.addPluginListener.mockReturnValue(new Promise((resolve) => (registered = resolve)))

    onNativeChromeIntent(vi.fn())()
    registered({ unregister: host.unregister })

    await vi.waitFor(() => expect(host.unregister).toHaveBeenCalledOnce())
  })

  it('listens to nothing without the phone’s chrome', () => {
    host.mobile = false
    onNativeChromeIntent(vi.fn())()
    expect(host.addPluginListener).not.toHaveBeenCalled()
  })
})
