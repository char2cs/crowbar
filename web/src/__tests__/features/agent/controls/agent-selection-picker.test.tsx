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

const providerA: AgentProvider = {
  id: 'provider-a',
  displayName: 'Provider A',
  icon: '<svg></svg>',
  connected: true,
  enabled: true,
  mcpEnabled: true,
  modelSelect: true,
  effortSelect: true,
  models: ['model-a', 'model-b', 'model-c'],
  efforts: {
    'model-a': ['low', 'medium', 'high'],
    'model-b': ['low', 'medium', 'high', 'xhigh', 'max'],
  },
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: false,
}

const providerB: AgentProvider = {
  id: 'provider-b',
  displayName: 'Provider B',
  icon: '<svg></svg>',
  connected: true,
  enabled: true,
  mcpEnabled: true,
  modelSelect: true,
  effortSelect: true,
  models: ['model-sol', 'model-luna'],
  efforts: { 'model-sol': ['low', 'medium', 'high', 'ultra'] },
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: false,
}

type PickerProps = Parameters<typeof AgentSelectionPicker>[0]
const draw = (props: Partial<PickerProps> = {}) =>
  render(
    <AgentSelectionPicker
      provider={providerA}
      providers={[providerA, providerB]}
      model="model-b"
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
    const noCatalogue: AgentProvider = { ...providerA, modelSelect: false, models: [] }
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
    expect(trigger).toHaveTextContent('model-b')
    expect(trigger).toHaveTextContent('High')
  })

  // A blank draft has neither: "Default · Default" said the same word twice.
  it('names an unpicked model and effort once', () => {
    draw({ providers: [providerA], model: '', effort: '', unsetLabel: 'Default' })
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent(/^Default$/)
  })

  it('still names the effort when only the model is unpicked', () => {
    draw({ providers: [providerA], model: '', effort: 'low', unsetLabel: 'Default' })
    expect(screen.getByRole('button', { name: /Agent:/ })).toHaveTextContent('DefaultLow')
  })

  it('groups models under their own provider section, one section per offered provider', () => {
    draw()
    openMenu()
    expect(screen.getByText('Provider A')).toBeInTheDocument()
    expect(screen.getByText('Provider B')).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'model-a' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'model-sol' })).toBeInTheDocument()
  })

  it('picking a model on the SAME provider stages provider, model and effort together', () => {
    const onSelectionChange = vi.fn()
    draw({ onSelectionChange })
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'model-a' }))

    // model-a declares high — the current effort survives the pick unchanged.
    expect(onSelectionChange).toHaveBeenCalledExactlyOnceWith('provider-a', 'model-a', 'high')
  })

  it('picking a model whose declared efforts do not include the current one falls back to its own first level', () => {
    const onSelectionChange = vi.fn()
    draw({ effort: 'xhigh', onSelectionChange })
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'model-a' }))

    // model-a has no xhigh — lands on model-a's own first declared level.
    expect(onSelectionChange).toHaveBeenCalledWith('provider-a', 'model-a', 'low')
  })

  // A cross-provider pick stages like any other; the switch happens server-side
  // with the next send, never live on the click.
  it('picking a model on a DIFFERENT provider only stages it — no live switch, nothing async', () => {
    const onSelectionChange = vi.fn()
    draw({ onSelectionChange })
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'model-sol' }))

    // model-sol declares high too, so the current effort survives the pick.
    expect(onSelectionChange).toHaveBeenCalledWith('provider-b', 'model-sol', 'high')
  })

  // A short catalogue fits on sight; a long one grows a search field.
  const crowded: AgentProvider = {
    ...providerA,
    models: [...(providerA.models ?? []), ...Array.from({ length: 10 }, (_, i) => `model-${i}`)],
  }

  it('offers no search field for a short catalogue', () => {
    draw()
    openMenu()
    expect(screen.queryByPlaceholderText(/Search models/)).not.toBeInTheDocument()
  })

  it('typing a provider name surfaces every model under it', () => {
    draw({ provider: crowded, providers: [crowded, providerB] })
    openMenu()
    fireEvent.change(screen.getByPlaceholderText(/Search models/), {
      target: { value: 'Provider B' },
    })

    expect(screen.getByText('Provider B')).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'model-sol' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'model-luna' })).toBeInTheDocument()
    expect(screen.queryByText('Provider A')).not.toBeInTheDocument()
  })

  it('filters to matching models only when the query names no provider', () => {
    draw({ provider: crowded, providers: [crowded, providerB] })
    openMenu()
    fireEvent.change(screen.getByPlaceholderText(/Search models/), { target: { value: 'model-c' } })

    expect(screen.getByRole('menuitem', { name: 'model-c' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'model-a' })).not.toBeInTheDocument()
    expect(screen.queryByText('Provider B')).not.toBeInTheDocument()
  })

  const effortGroup = () => screen.getByRole('radiogroup', { name: /Reasoning effort/ })
  const checkedEffort = () =>
    within(effortGroup())
      .getAllByRole('radio')
      .filter((radio) => radio.getAttribute('aria-checked') === 'true')
      .map((radio) => radio.textContent)

  it('lists the efforts of the CURRENT model, not the whole provider', () => {
    draw({ provider: providerB, model: 'model-sol' })
    openMenu()
    expect(
      within(effortGroup())
        .getAllByRole('radio')
        .map((radio) => radio.textContent),
    ).toEqual(['Low', 'Medium', 'High', 'Ultra'])
  })

  // An undeclared effort must not clamp to "Low": nobody picked that level.
  it('marks no effort when the current one is not declared by the model', () => {
    draw({ provider: providerB, model: 'model-sol', effort: 'Default' })
    openMenu()
    expect(checkedEffort()).toEqual([])
    expect(screen.getByRole('radio', { name: 'Low' })).toBeInTheDocument()
  })

  // '' is "the provider's default": with that default stated, it is the level marked.
  it("marks the model's stated default effort when none is chosen", () => {
    const withDefault: AgentProvider = { ...providerB, defaultEfforts: { 'model-sol': 'medium' } }
    draw({
      provider: withDefault,
      providers: [providerA, withDefault],
      model: 'model-sol',
      effort: '',
    })
    openMenu()
    expect(checkedEffort()).toEqual(['Medium'])
  })

  it('hides the effort control entirely for a model with no declared levels', () => {
    draw({ provider: providerB, model: 'model-luna', effort: '' })
    openMenu()
    expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
  })

  it('picking an effort changes only the effort, keeping the model', () => {
    const onSelectionChange = vi.fn()
    draw({ onSelectionChange })
    openMenu()
    fireEvent.click(screen.getByRole('radio', { name: 'Max' }))

    expect(onSelectionChange).toHaveBeenCalledWith('provider-a', 'model-b', 'max')
  })

  it('marks the current model as picked (a checkmark) and no other row', () => {
    draw()
    openMenu()
    const rowB = screen.getByRole('menuitem', { name: 'model-b' })
    const rowA = screen.getByRole('menuitem', { name: 'model-a' })
    expect(rowB.querySelector('svg')).toBeTruthy()
    expect(rowA.querySelector('svg')).toBeNull()
  })

  it('shows nothing selected when model/effort are unset — the "unfired hook" state, not a bug', () => {
    draw({ model: '', effort: '' })
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveAccessibleName('Agent: Provider A, model unset, effort unset')
    openMenu()
    for (const name of ['model-a', 'model-b', 'model-c']) {
      expect(screen.getByRole('menuitem', { name }).querySelector('svg')).toBeNull()
    }
  })

  // `efforts['']` is the backend's levels for an unselected model, so the raw
  // unset model ('') must find it, where a baked-in label would not.
  it("finds the provider's own '' entry and draws the effort control when no model is confirmed yet", () => {
    const withDefaultEfforts: AgentProvider = {
      ...providerA,
      efforts: { ...providerA.efforts, '': ['low', 'medium'] },
    }
    draw({
      provider: withDefaultEfforts,
      providers: [withDefaultEfforts, providerB],
      model: '',
      effort: '',
    })
    openMenu()
    expect(
      within(effortGroup())
        .getAllByRole('radio')
        .map((radio) => radio.textContent),
    ).toEqual(['Low', 'Medium'])
  })

  // Without an '' entry there are no levels until a model is picked: absent, not disabled.
  it('renders no effort control for an unconfirmed model when the provider declares no own-default entry', () => {
    draw({ model: '', effort: '' })
    openMenu()
    expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
  })

  // `reportedModel` names which model answered; it is display-only, never the selection.
  it('shows the provider-reported model next to the unset label when nothing is explicitly picked', () => {
    draw({ model: '', effort: '', reportedModel: 'Model X 4.5' })
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent('unset')
    expect(trigger).toHaveTextContent('(Model X 4.5)')
    expect(trigger).toHaveAccessibleName(
      'Agent: Provider A, model unset (reported Model X 4.5), effort unset',
    )
  })

  it('an explicit model selection ignores a reported model entirely', () => {
    draw({ reportedModel: 'Model X 4.5' })
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent('model-b')
    expect(trigger).not.toHaveTextContent('Model X')
    expect(trigger).toHaveAccessibleName('Agent: Provider A, model model-b, effort high')
  })

  // The report must not stand in for the '' model: the effort control still finds
  // `efforts['']`, and a pick stages the raw '' model.
  it("a reported model does not break the unset model's own efforts[''] control or leak into a pick", () => {
    const withDefaultEfforts: AgentProvider = {
      ...providerA,
      efforts: { ...providerA.efforts, '': ['low', 'medium'] },
    }
    const onSelectionChange = vi.fn()
    draw({
      provider: withDefaultEfforts,
      providers: [withDefaultEfforts, providerB],
      model: '',
      effort: '',
      reportedModel: 'Model X 4.5',
      onSelectionChange,
    })
    openMenu()
    expect(screen.getByRole('radio', { name: 'Low' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('radio', { name: 'Medium' }))
    expect(onSelectionChange).toHaveBeenCalledWith('provider-a', '', 'medium')
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
      draw({ model: 'model-a', effort: 'medium', onSelectionChange })
      return onSelectionChange
    }

    // The menu is a native sheet there: the page hands it the catalogue and draws no popover.
    it('opens the native picker with the catalogue, and no menu of its own', async () => {
      renderPhone()
      openMenu()
      await waitFor(() => expect(last().open).toBe(true))
      expect(screen.queryByRole('menuitem')).not.toBeInTheDocument()
      expect(last()).toMatchObject({
        selected: { providerId: 'provider-a', modelId: 'model-a' },
        efforts: ['low', 'medium', 'high'],
        effort: 'medium',
      })
      expect(last().providers.map((p) => [p.name, p.models.map((m) => m.id)])).toEqual([
        ['Provider A', ['model-a', 'model-b', 'model-c']],
        ['Provider B', ['model-sol', 'model-luna']],
      ])
    })

    it('stages a model picked natively, under its own provider, and closes', async () => {
      const onSelectionChange = renderPhone()
      openMenu()
      await waitFor(() => expect(last().open).toBe(true))
      send({ type: 'selectModel', providerId: 'provider-b', modelId: 'model-sol' })
      expect(onSelectionChange).toHaveBeenCalledWith('provider-b', 'model-sol', 'medium')
      await waitFor(() => expect(last().open).toBe(false))
    })

    it('stages an effort picked natively and stays open', async () => {
      const onSelectionChange = renderPhone()
      openMenu()
      await waitFor(() => expect(last().open).toBe(true))
      send({ type: 'setEffort', effort: 'high' })
      expect(onSelectionChange).toHaveBeenCalledWith('provider-a', 'model-a', 'high')
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
