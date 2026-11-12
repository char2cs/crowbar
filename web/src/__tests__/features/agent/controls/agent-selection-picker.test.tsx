import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { NativeChromeIntent, NativeModelPicker } from '@/lib/native-chrome'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'

const chrome = vi.hoisted(() => ({
  on: false,
  pushed: [] as NativeModelPicker[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
}))
vi.mock('@/lib/native-chrome', () => ({
  hasNativeChrome: () => chrome.on,
  setNativeModelPicker: (picker: NativeModelPicker) => chrome.pushed.push(picker),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))
const setLevel = vi.hoisted(() => vi.fn().mockResolvedValue(undefined))
vi.mock('@/features/agent/api/agent-chat-commands', () => ({ setChatPermissionLevel: setLevel }))

const { AgentSelectionPicker } = await import('@/features/agent/controls/agent-selection-picker')
const { openModelPicker } = await import('@/features/agent/controls/model-picker-open')

const claude: AgentProvider = {
  id: 'claude',
  displayName: 'Claude',
  icon: '<svg></svg>',
  connected: true,
  enabled: true,
  mcpEnabled: true,
  modelSelect: true,
  effortSelect: true,
  models: ['sonnet', 'opus', 'haiku'],
  efforts: { sonnet: ['low', 'medium', 'high'], opus: ['low', 'medium', 'high', 'xhigh', 'max'] },
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: false,
}

const codex: AgentProvider = {
  id: 'codex',
  displayName: 'Codex',
  icon: '<svg></svg>',
  connected: true,
  enabled: true,
  mcpEnabled: true,
  modelSelect: true,
  effortSelect: true,
  models: ['gpt-5.6-sol', 'gpt-5.6-luna'],
  efforts: { 'gpt-5.6-sol': ['low', 'medium', 'high', 'ultra'] },
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: false,
}

type PickerProps = Parameters<typeof AgentSelectionPicker>[0]
const draw = (props: Partial<PickerProps> = {}) =>
  render(
    <AgentSelectionPicker
      provider={claude}
      providers={[claude, codex]}
      model="opus"
      effort="high"
      onSelectionChange={vi.fn()}
      {...props}
    />,
  )

function openMenu() {
  fireEvent.click(screen.getByRole('button', { name: /Agent:/ }))
}

describe('AgentSelectionPicker', () => {
  // The open state is shared by chat id, so a test that leaves the menu open would toggle it shut
  // for the next one.
  afterEach(() => {
    const trigger = screen.queryByRole('button', { name: /Agent:/ })
    if (trigger?.getAttribute('aria-expanded') === 'true') fireEvent.click(trigger)
  })

  it('renders nothing when no offered provider declares a model catalogue', () => {
    const noCatalogue: AgentProvider = { ...claude, modelSelect: false, models: [] }
    const { container } = draw({
      provider: noCatalogue,
      providers: [noCatalogue],
      model: '',
      effort: '',
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the current provider icon, model and effort on the trigger', () => {
    draw()
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent('opus')
    expect(trigger).toHaveTextContent('High')
  })

  // A blank draft has neither: "Default · Default" said the same word twice.
  it('names an unpicked model and effort once', () => {
    draw({ providers: [claude], model: '', effort: '', unsetLabel: 'Default' })
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent(/^Default$/)
  })

  it('still names the effort when only the model is unpicked', () => {
    draw({ providers: [claude], model: '', effort: 'low', unsetLabel: 'Default' })
    expect(screen.getByRole('button', { name: /Agent:/ })).toHaveTextContent('DefaultLow')
  })

  it('groups models under their own provider section, one section per offered provider', () => {
    draw()
    openMenu()
    expect(screen.getByText('Claude')).toBeInTheDocument()
    expect(screen.getByText('Codex')).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'sonnet' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'gpt-5.6-sol' })).toBeInTheDocument()
  })

  it('picking a model on the SAME provider stages provider, model and effort together', () => {
    const onSelectionChange = vi.fn()
    draw({ onSelectionChange })
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'sonnet' }))

    // sonnet declares high — the current effort survives the pick unchanged.
    expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith('claude', 'sonnet', 'high')
  })

  it('picking a model whose declared efforts do not include the current one falls back to its own first level', () => {
    const onSelectionChange = vi.fn()
    draw({ effort: 'xhigh', onSelectionChange })
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'sonnet' }))

    // sonnet has no xhigh — lands on sonnet's own first declared level.
    expect(onSelectionChange).toHaveBeenCalledWith('claude', 'sonnet', 'low')
  })

  // A cross-provider pick stages like any other; the switch happens server-side
  // with the next send, never live on the click.
  it('picking a model on a DIFFERENT provider only stages it — no live switch, nothing async', () => {
    const onSelectionChange = vi.fn()
    draw({ onSelectionChange })
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'gpt-5.6-sol' }))

    // gpt-5.6-sol declares high too, so the current effort survives the pick.
    expect(onSelectionChange).toHaveBeenCalledWith('codex', 'gpt-5.6-sol', 'high')
  })

  it('typing a provider name surfaces every model under it', () => {
    draw()
    openMenu()
    fireEvent.change(screen.getByPlaceholderText(/Search models/), { target: { value: 'codex' } })

    expect(screen.getByText('Codex')).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'gpt-5.6-sol' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'gpt-5.6-luna' })).toBeInTheDocument()
    expect(screen.queryByText('Claude')).not.toBeInTheDocument()
  })

  it('filters to matching models only when the query names no provider', () => {
    draw()
    openMenu()
    fireEvent.change(screen.getByPlaceholderText(/Search models/), { target: { value: 'haiku' } })

    expect(screen.getByRole('menuitem', { name: 'haiku' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'sonnet' })).not.toBeInTheDocument()
    expect(screen.queryByText('Codex')).not.toBeInTheDocument()
  })

  it('renders the effort slider for the CURRENT model, not the whole provider', () => {
    draw({ provider: codex, model: 'gpt-5.6-sol' })
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    const effortSection = slider.parentElement?.parentElement as HTMLElement
    expect(within(effortSection).getByText('Ultra')).toBeInTheDocument()
  })

  // An undeclared effort must not clamp to "Low": nobody picked that level.
  it('draws no thumb/fill for an effort the current model does not declare', () => {
    draw({ provider: codex, model: 'gpt-5.6-sol', effort: 'Default' })
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    expect(slider).not.toHaveAttribute('aria-valuenow')
    expect(slider.querySelector('.bg-primary')).not.toBeInTheDocument()
    // A real pick is still possible; only nothing is highlighted as chosen.
    expect(screen.getByRole('button', { name: 'Low' })).toBeInTheDocument()
  })

  // '' is "the provider's default": with that default stated, the slider shows the level it is.
  it("places the thumb on the model's stated default effort when none is chosen", () => {
    const onSelectionChange = vi.fn()
    const withDefault: AgentProvider = { ...codex, defaultEfforts: { 'gpt-5.6-sol': 'medium' } }
    draw({
      provider: withDefault,
      providers: [claude, withDefault],
      model: 'gpt-5.6-sol',
      effort: '',
      onSelectionChange,
    })
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    expect(slider).toHaveAttribute('aria-valuenow', '1')
    expect(slider).toHaveAttribute('aria-valuetext', 'Medium (default)')
    fireEvent.keyDown(slider, { key: 'ArrowRight' })
    expect(onSelectionChange).toHaveBeenLastCalledWith('codex', 'gpt-5.6-sol', 'high')
  })

  // Tracking rides `window` pointer listeners, not Pointer Capture, which throws
  // in WKWebView; so moves fire on `window` and capture is never stubbed.
  it('is an actually-draggable slider, not just a click-to-jump bar', () => {
    const onSelectionChange = vi.fn()
    draw({ effort: 'low', onSelectionChange })
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    // opus: low, medium, high, xhigh, max — 5 stops across the track width.
    slider.getBoundingClientRect = () =>
      ({ left: 0, right: 100, width: 100, top: 0, bottom: 0, height: 0 }) as DOMRect

    fireEvent.pointerDown(slider, { clientX: 0, pointerId: 1 })
    expect(onSelectionChange).toHaveBeenLastCalledWith('claude', 'opus', 'low')

    fireEvent.pointerMove(window, { clientX: 100, pointerId: 1 })
    expect(onSelectionChange).toHaveBeenLastCalledWith('claude', 'opus', 'max')

    fireEvent.pointerMove(window, { clientX: 50, pointerId: 1 })
    expect(onSelectionChange).toHaveBeenLastCalledWith('claude', 'opus', 'high')

    fireEvent.pointerUp(window, { pointerId: 1 })
    // A move AFTER release must not keep tracking.
    fireEvent.pointerMove(window, { clientX: 0, pointerId: 1 })
    expect(onSelectionChange).toHaveBeenLastCalledWith('claude', 'opus', 'high')
  })

  it('hides the effort slider entirely for a model with no declared levels', () => {
    draw({ provider: codex, model: 'gpt-5.6-luna', effort: '' })
    openMenu()
    expect(screen.queryByRole('slider')).not.toBeInTheDocument()
  })

  it('clicking an effort tick changes only the effort, keeping the model', () => {
    const onSelectionChange = vi.fn()
    draw({ onSelectionChange })
    openMenu()
    fireEvent.click(screen.getByRole('button', { name: 'Max' }))

    expect(onSelectionChange).toHaveBeenCalledWith('claude', 'opus', 'max')
  })

  it('marks the current model as picked (a checkmark) and no other row', () => {
    draw()
    openMenu()
    const opusRow = screen.getByRole('menuitem', { name: 'opus' })
    const sonnetRow = screen.getByRole('menuitem', { name: 'sonnet' })
    expect(opusRow.querySelector('svg')).toBeTruthy()
    expect(sonnetRow.querySelector('svg')).toBeNull()
  })

  it('shows nothing selected when model/effort are unset — the "unfired hook" state, not a bug', () => {
    draw({ model: '', effort: '' })
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveAccessibleName('Agent: Claude, model unset, effort unset')
    openMenu()
    for (const name of ['sonnet', 'opus', 'haiku']) {
      expect(screen.getByRole('menuitem', { name }).querySelector('svg')).toBeNull()
    }
  })

  // `efforts['']` is the backend's levels for an unselected model, so the raw
  // unset model ('') must find it, where a baked-in label would not.
  it("finds the provider's own '' entry and draws the slider when no model is confirmed yet", () => {
    const withDefaultEfforts: AgentProvider = {
      ...claude,
      efforts: { ...claude.efforts, '': ['low', 'medium'] },
    }
    draw({
      provider: withDefaultEfforts,
      providers: [withDefaultEfforts, codex],
      model: '',
      effort: '',
    })
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    const effortSection = slider.parentElement?.parentElement as HTMLElement
    expect(within(effortSection).getByText('Low')).toBeInTheDocument()
    expect(within(effortSection).getByText('Medium')).toBeInTheDocument()
  })

  // Without an '' entry there are no levels until a model is picked: absent, not disabled.
  it('renders no slider for an unconfirmed model when the provider declares no own-default entry', () => {
    draw({ model: '', effort: '' })
    openMenu()
    expect(screen.queryByRole('slider')).not.toBeInTheDocument()
  })

  // `reportedModel` names which model answered; it is display-only, never the selection.
  it('names the provider-reported model, not the unset label, when nothing is explicitly picked', () => {
    draw({ model: '', effort: '', reportedModel: 'Claude Sonnet 4.5' })
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent('Claude Sonnet 4.5')
    expect(trigger).not.toHaveTextContent('(')
    expect(trigger).toHaveAccessibleName('Agent: Claude, model Claude Sonnet 4.5, effort unset')
  })

  it('an explicit model selection ignores a reported model entirely', () => {
    draw({ reportedModel: 'Claude Sonnet 4.5' })
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent('opus')
    expect(trigger).not.toHaveTextContent('Sonnet')
    expect(trigger).toHaveAccessibleName('Agent: Claude, model opus, effort high')
  })

  // The report must not stand in for the '' model: the slider still finds
  // `efforts['']`, and a pick stages the raw '' model.
  it("a reported model does not break the unset model's own efforts[''] slider or leak into a pick", () => {
    const withDefaultEfforts: AgentProvider = {
      ...claude,
      efforts: { ...claude.efforts, '': ['low', 'medium'] },
    }
    const onSelectionChange = vi.fn()
    draw({
      provider: withDefaultEfforts,
      providers: [withDefaultEfforts, codex],
      model: '',
      effort: '',
      reportedModel: 'Claude Sonnet 4.5',
      onSelectionChange,
    })
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    const effortSection = slider.parentElement?.parentElement as HTMLElement
    expect(within(effortSection).getByText('Low')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Medium' }))
    expect(onSelectionChange).toHaveBeenCalledWith('claude', '', 'medium')
  })

  it('lights the chip when told the way on is another model, and the menu not at all', () => {
    draw({ hot: true })
    expect(screen.getByRole('button', { name: /Agent:/ })).toHaveAttribute('data-hot', 'true')
    openMenu()
    expect(document.querySelector('[role="menu"] [data-hot]')).toBeNull()
  })

  it('opens from outside for its chat, as a notice’s Switch model does', () => {
    draw({ permission: { wsId: 'w1', chatId: 'c-open', levels: ['guarded'] } })
    act(() => openModelPicker('c-open'))
    expect(screen.getByRole('button', { name: /Agent:/ })).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(screen.getByRole('button', { name: /Agent:/ }))
  })

  describe('on the phone', () => {
    const send = (intent: NativeChromeIntent) =>
      act(() => chrome.listeners.forEach((listener) => listener(intent)))
    const last = () => chrome.pushed.at(-1)!
    afterEach(() => {
      chrome.on = false
      chrome.pushed.length = 0
      chrome.listeners.clear()
    })
    const renderPhone = (onSelectionChange = vi.fn()) => {
      chrome.on = true
      draw({ model: 'sonnet', effort: 'medium', onSelectionChange })
      return onSelectionChange
    }

    // The menu is a native sheet there: the page hands it the catalogue and draws no popover.
    it('opens the native picker with the catalogue, and no menu of its own', async () => {
      renderPhone()
      openMenu()
      await waitFor(() => expect(last().open).toBe(true))
      expect(screen.queryByRole('menuitem')).not.toBeInTheDocument()
      expect(last()).toMatchObject({
        selected: { providerId: 'claude', modelId: 'sonnet' },
        efforts: ['low', 'medium', 'high'],
        effort: 'medium',
      })
      expect(last().providers.map((p) => [p.name, p.models.map((m) => m.id)])).toEqual([
        ['Claude', ['sonnet', 'opus', 'haiku']],
        ['Codex', ['gpt-5.6-sol', 'gpt-5.6-luna']],
      ])
    })

    it('stages a model picked natively, under its own provider, and closes', async () => {
      const onSelectionChange = renderPhone()
      openMenu()
      await waitFor(() => expect(last().open).toBe(true))
      send({ type: 'selectModel', providerId: 'codex', modelId: 'gpt-5.6-sol' })
      expect(onSelectionChange).toHaveBeenCalledWith('codex', 'gpt-5.6-sol', 'medium')
      await waitFor(() => expect(last().open).toBe(false))
    })

    it('stages an effort picked natively and stays open', async () => {
      const onSelectionChange = renderPhone()
      openMenu()
      await waitFor(() => expect(last().open).toBe(true))
      send({ type: 'setEffort', effort: 'high' })
      expect(onSelectionChange).toHaveBeenCalledWith('claude', 'sonnet', 'high')
      expect(last().open).toBe(true)
    })
  })

  describe('permission level on the phone', () => {
    const permission = {
      wsId: 'w1',
      chatId: 'c1',
      levels: ['guarded' as const, 'trusted' as const],
    }
    const send = (intent: NativeChromeIntent) =>
      act(() => chrome.listeners.forEach((listener) => listener(intent)))
    beforeEach(() => {
      resetChatStore()
      writeChat({ id: 'c1', permissionLevel: 'trusted' })
      setLevel.mockClear()
    })
    afterEach(() => {
      chrome.on = false
      platform.mobile = false
      chrome.pushed.length = 0
      chrome.listeners.clear()
    })

    it('hands the native sheet the declared levels and the current one', async () => {
      chrome.on = true
      platform.mobile = true
      draw({ permission })
      openMenu()
      await waitFor(() => expect(chrome.pushed.at(-1)?.open).toBe(true))
      expect(chrome.pushed.at(-1)?.permission).toEqual({
        levels: [
          {
            value: 'guarded',
            label: 'Guarded',
            description: 'Asks before edits, commands and tools.',
          },
          {
            value: 'trusted',
            label: 'Trusted',
            description: 'Edits files freely. Asks before commands.',
          },
        ],
        level: 'trusted',
      })
    })

    it('stages a draft pick instead of writing it', async () => {
      chrome.on = true
      platform.mobile = true
      const onPick = vi.fn()
      const draft = { hostId: 'local', level: 'guarded' as const, onPick }
      draw({ permission: { wsId: 'w1', chatId: '', levels: ['guarded', 'trusted'], draft } })
      openMenu()
      await waitFor(() => expect(chrome.pushed.at(-1)?.permission?.level).toBe('guarded'))
      send({ type: 'setPermissionLevel', level: 'trusted' })
      expect(onPick).toHaveBeenCalledWith('trusted')
      expect(setLevel).not.toHaveBeenCalled()
    })

    it('writes a level picked natively and keeps the sheet open', async () => {
      chrome.on = true
      platform.mobile = true
      draw({ permission })
      openMenu()
      await waitFor(() => expect(chrome.pushed.at(-1)?.open).toBe(true))
      send({ type: 'setPermissionLevel', level: 'guarded' })
      expect(setLevel).toHaveBeenCalledWith('w1', 'c1', 'guarded')
      expect(chrome.pushed.at(-1)?.open).toBe(true)
    })

    it('offers the same choice in the web menu where there is no native sheet', () => {
      platform.mobile = true
      draw({ permission })
      openMenu()
      fireEvent.click(screen.getByRole('menuitemradio', { name: 'Guarded' }))
      expect(setLevel).toHaveBeenCalledWith('w1', 'c1', 'guarded')
    })

    it('offers none on the desktop, where the footer chip owns it', () => {
      draw({ permission })
      openMenu()
      expect(screen.queryByRole('menuitemradio')).not.toBeInTheDocument()
    })
  })
})
