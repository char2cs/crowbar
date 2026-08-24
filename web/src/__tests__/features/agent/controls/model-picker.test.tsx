import { createElement, forwardRef, useState, type ReactNode } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import {
  AgentModelPicker,
  type AgentModelPickerProps,
} from '@/features/agent/controls/model-picker'

const { setSelectionFn } = vi.hoisted(() => ({ setSelectionFn: vi.fn() }))

vi.mock('@/features/agent/api/agent-api', () => ({
  setChatSelection: (...args: unknown[]) => setSelectionFn(...args),
}))

// The shared Dropdown animates open/close through AnimatePresence, which keeps an
// exiting menu mounted until a transition jsdom never fires — a closed menu would
// only be observable through a timing wait. This synchronous stand-in makes closing
// immediate, with no timers and no polling.
vi.mock('framer-motion', () => {
  const MotionDiv = forwardRef<HTMLDivElement, Record<string, unknown>>(function MotionDiv(
    { initial: _initial, animate: _animate, exit: _exit, transition: _transition, ...domProps },
    ref,
  ) {
    return createElement('div', { ref, ...domProps })
  })
  return {
    motion: new Proxy({} as Record<string, unknown>, { get: () => MotionDiv }),
    m: new Proxy({} as Record<string, unknown>, { get: () => MotionDiv }),
    AnimatePresence: ({ children }: { children?: ReactNode }) => children ?? null,
    LazyMotion: ({ children }: { children?: ReactNode }) => children ?? null,
    domAnimation: {},
  }
})

// Codex's real shape, trimmed: `ultra` exists on gpt-5.6-sol and stops at `max` on
// gpt-5.6-luna, which is the pair that can strand an invalid level. '' is the key
// for the provider's OWN default model — the backend resolves it, never the client.
function provider(overrides: Partial<AgentProvider> = {}): AgentProvider {
  return {
    id: 'codex',
    displayName: 'Codex',
    icon: '',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    modelSelect: true,
    effortSelect: true,
    models: ['gpt-5.6-sol', 'gpt-5.6-luna', 'gpt-5.4-mini'],
    efforts: {
      '': ['low', 'medium', 'high'],
      'gpt-5.6-sol': ['low', 'medium', 'high', 'max', 'ultra'],
      'gpt-5.6-luna': ['low', 'medium', 'high', 'max'],
    },
    ...overrides,
  }
}

function setup(overrides: Partial<AgentModelPickerProps> = {}) {
  const onSelectionChange = vi.fn()
  const props: AgentModelPickerProps = {
    wsId: 'w1',
    chatId: 'c1',
    provider: provider(),
    model: '',
    effort: '',
    onSelectionChange,
    ...overrides,
  }
  return { onSelectionChange, ...render(<AgentModelPicker {...props} />) }
}

/**
 * The picker as the app actually wires it: the OWNER of the chat holds the
 * selection and feeds an accepted pair back down. Without that loop the control
 * paints its optimistic value and then reverts, which is a property of the test
 * fixture rather than of the component.
 */
function setupLive(overrides: Partial<AgentModelPickerProps> = {}) {
  function Harness() {
    const [selection, setSelection] = useState({ model: '', effort: '' })
    return (
      <AgentModelPicker
        wsId="w1"
        chatId="c1"
        provider={provider()}
        {...overrides}
        model={overrides.model ?? selection.model}
        effort={overrides.effort ?? selection.effort}
        onSelectionChange={(model, effort) => setSelection({ model, effort })}
      />
    )
  }
  return render(<Harness />)
}

const modelTrigger = () => screen.getByRole('button', { name: /^Model:/ })
const effortTrigger = () => screen.getByRole('button', { name: /^Effort:/ })
const queryEffortTrigger = () => screen.queryByRole('button', { name: /^Effort:/ })
const item = (name: string) => screen.getByRole('menuitem', { name })
const ticked = (name: string) => within(item(name)).queryByTestId('selection-tick') !== null

describe('AgentModelPicker', () => {
  beforeEach(() => {
    setSelectionFn.mockReset()
    setSelectionFn.mockResolvedValue(undefined)
  })

  describe('when the capability is absent', () => {
    it('renders NOTHING for a provider that declares neither catalogue', () => {
      // Absent capability is absent UI. A disabled control would say "broken";
      // this provider simply does not offer the choice.
      setup({ provider: provider({ modelSelect: false, effortSelect: false }) })
      expect(screen.queryByTestId('agent-model-picker')).toBeNull()
      expect(screen.queryAllByRole('button')).toHaveLength(0)
    })

    it('renders no MODEL control when only effort selection is declared', () => {
      setup({ provider: provider({ modelSelect: false }) })
      expect(screen.queryByRole('button', { name: /^Model:/ })).toBeNull()
      expect(effortTrigger()).toBeTruthy()
    })

    it('renders no EFFORT control when only model selection is declared', () => {
      setup({ provider: provider({ effortSelect: false }) })
      expect(modelTrigger()).toBeTruthy()
      expect(queryEffortTrigger()).toBeNull()
    })

    it('renders nothing at all for a chat whose provider is not known yet', () => {
      setup({ provider: undefined })
      expect(screen.queryByTestId('agent-model-picker')).toBeNull()
    })

    it('renders no effort control for a model the provider declares NO levels for', () => {
      // A model with no levels is absent from the map entirely — draw nothing,
      // rather than an empty menu that implies the levels are merely missing.
      setup({ model: 'gpt-5.4-mini' })
      expect(modelTrigger()).toBeTruthy()
      expect(queryEffortTrigger()).toBeNull()
    })
  })

  describe('when the chat has made no selection', () => {
    it('names the default instead of inventing a model', () => {
      setup()
      expect(modelTrigger()).toHaveTextContent('Default model')
      expect(effortTrigger()).toHaveTextContent('Default effort')
      // Not any catalogue entry, and not the first one either.
      expect(modelTrigger()).not.toHaveTextContent('gpt-5.6-sol')
      // The accessible name still says WHOSE default it is.
      expect(modelTrigger().getAttribute('aria-label')).toBe('Model: Provider default')
    })

    it('marks NOTHING in the catalogue as selected', () => {
      setup()
      fireEvent.click(modelTrigger())

      expect(ticked('Provider default')).toBe(true)
      for (const model of ['gpt-5.6-sol', 'gpt-5.6-luna', 'gpt-5.4-mini']) {
        expect(ticked(model)).toBe(false)
      }
    })

    it("offers the levels of the provider's own default model — efforts['']", () => {
      // The fallback is applied server-side; the client just reads the '' key.
      setup()
      fireEvent.click(effortTrigger())

      expect(screen.getAllByRole('menuitem').map((el) => el.textContent)).toEqual([
        'Provider default',
        'low',
        'medium',
        'high',
      ])
    })

    it('lists the catalogue in DESCRIPTOR order, never re-sorted', () => {
      setup()
      fireEvent.click(modelTrigger())

      expect(screen.getAllByRole('menuitem').map((el) => el.textContent)).toEqual([
        'Provider default',
        'gpt-5.6-sol',
        'gpt-5.6-luna',
        'gpt-5.4-mini',
      ])
    })
  })

  describe('when the chat HAS a selection', () => {
    it('shows it on both triggers and ticks it in the menus', () => {
      setup({ model: 'gpt-5.6-luna', effort: 'max' })
      expect(modelTrigger()).toHaveTextContent('gpt-5.6-luna')
      expect(effortTrigger()).toHaveTextContent('max')

      fireEvent.click(modelTrigger())
      expect(ticked('gpt-5.6-luna')).toBe(true)
      expect(ticked('Provider default')).toBe(false)
    })

    it("reads the SELECTED model's levels, not another model's", () => {
      setup({ model: 'gpt-5.6-luna', effort: 'max' })
      fireEvent.click(effortTrigger())

      // gpt-5.6-sol has `ultra`; luna stops at `max`.
      expect(screen.queryByRole('menuitem', { name: 'ultra' })).toBeNull()
      expect(item('max')).toBeTruthy()
    })
  })

  describe('writing a selection', () => {
    it('sends BOTH halves and commits the accepted pair upward', async () => {
      const { onSelectionChange } = setup({ model: 'gpt-5.6-sol', effort: 'high' })
      fireEvent.click(effortTrigger())
      fireEvent.click(item('max'))

      await waitFor(() => expect(onSelectionChange).toHaveBeenCalledWith('gpt-5.6-sol', 'max'))
      expect(setSelectionFn).toHaveBeenCalledWith('w1', 'c1', 'gpt-5.6-sol', 'max')
    })

    it('CLEARS an effort the new model does not declare, in the SAME write', async () => {
      // gpt-5.6-sol supports `ultra`; gpt-5.6-luna stops at `max`. Sending the pair
      // as-is would ask for a combination that was never jointly valid.
      const { onSelectionChange } = setup({ model: 'gpt-5.6-sol', effort: 'ultra' })
      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.6-luna'))

      await waitFor(() => expect(setSelectionFn).toHaveBeenCalledTimes(1))
      expect(setSelectionFn).toHaveBeenCalledWith('w1', 'c1', 'gpt-5.6-luna', '')
      expect(onSelectionChange).toHaveBeenCalledWith('gpt-5.6-luna', '')
    })

    it('KEEPS an effort the new model still declares', async () => {
      const { onSelectionChange } = setup({ model: 'gpt-5.6-sol', effort: 'high' })
      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.6-luna'))

      await waitFor(() => expect(onSelectionChange).toHaveBeenCalledWith('gpt-5.6-luna', 'high'))
      expect(setSelectionFn).toHaveBeenCalledWith('w1', 'c1', 'gpt-5.6-luna', 'high')
    })

    it('clears an effort a model with NO declared levels cannot support', async () => {
      setup({ model: 'gpt-5.6-sol', effort: 'high' })
      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.4-mini'))

      await waitFor(() =>
        expect(setSelectionFn).toHaveBeenCalledWith('w1', 'c1', 'gpt-5.4-mini', ''),
      )
    })

    it("sends '' to go back to the provider's default MODEL", async () => {
      const { onSelectionChange } = setup({ model: 'gpt-5.6-sol', effort: 'high' })
      fireEvent.click(modelTrigger())
      fireEvent.click(item('Provider default'))

      // 'high' survives: the default model declares it too (efforts['']).
      await waitFor(() => expect(setSelectionFn).toHaveBeenCalledWith('w1', 'c1', '', 'high'))
      expect(onSelectionChange).toHaveBeenCalledWith('', 'high')
    })

    it("sends '' to go back to the provider's default EFFORT", async () => {
      const { onSelectionChange } = setup({ model: 'gpt-5.6-luna', effort: 'max' })
      fireEvent.click(effortTrigger())
      fireEvent.click(item('Provider default'))

      await waitFor(() =>
        expect(setSelectionFn).toHaveBeenCalledWith('w1', 'c1', 'gpt-5.6-luna', ''),
      )
      expect(onSelectionChange).toHaveBeenCalledWith('gpt-5.6-luna', '')
    })

    it("clears BOTH halves when the default model does not declare the chat's effort", async () => {
      // efforts[''] is low/medium/high — `ultra` cannot come along.
      setup({ model: 'gpt-5.6-sol', effort: 'ultra' })
      fireEvent.click(modelTrigger())
      fireEvent.click(item('Provider default'))

      await waitFor(() => expect(setSelectionFn).toHaveBeenCalledWith('w1', 'c1', '', ''))
    })

    it('paints the new choice immediately, without waiting for the round trip', async () => {
      let settle = () => {}
      setSelectionFn.mockReturnValue(new Promise<void>((resolve) => (settle = () => resolve())))
      setup({ model: 'gpt-5.6-sol', effort: 'high' })

      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.6-luna'))

      expect(modelTrigger()).toHaveTextContent('gpt-5.6-luna')
      // And the control is closed to further writes until this one lands.
      expect(modelTrigger()).toBeDisabled()

      settle()
      await waitFor(() => expect(modelTrigger()).not.toBeDisabled())
    })
  })

  describe('when the server refuses', () => {
    it('says a rejected value is no longer offered, and does NOT commit upward', async () => {
      setSelectionFn.mockRejectedValue(new ApiError('model not in catalogue', 400))
      const { onSelectionChange } = setup({ model: 'gpt-5.6-sol', effort: 'high' })

      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.6-luna'))

      const alert = await screen.findByRole('alert')
      expect(alert.textContent).toMatch(/no longer offers that combination/i)
      expect(onSelectionChange).not.toHaveBeenCalled()
      // The refused value does not stick: the trigger falls back to the truth.
      expect(modelTrigger()).toHaveTextContent('gpt-5.6-sol')
    })

    it('says a chat with no provider yet has nothing to configure', async () => {
      setSelectionFn.mockRejectedValue(new ApiError('chat has no provider', 422))
      setup()

      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.6-sol'))

      const alert = await screen.findByRole('alert')
      expect(alert.textContent).toMatch(/no provider yet/i)
    })

    it('shows any other failure verbatim rather than swallowing it', async () => {
      setSelectionFn.mockRejectedValue(new Error('daemon unreachable'))
      setup()

      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.6-sol'))

      expect((await screen.findByRole('alert')).textContent).toBe('daemon unreachable')
    })

    it('clears a stale error when the next write is attempted', async () => {
      setSelectionFn.mockRejectedValueOnce(new ApiError('nope', 400))
      setup()

      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.6-sol'))
      await screen.findByRole('alert')

      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.6-luna'))
      await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
    })
  })

  describe("when the chat's stored selection is off this provider's catalogue", () => {
    // A provider switch keeps the chat's sticky choice — the two are independent —
    // so a chat moved from claude to codex still holds `sonnet`.
    it('goes on naming what the chat asked for, and says the provider does not offer it', () => {
      setup({ model: 'sonnet', effort: 'high' })

      expect(modelTrigger()).toHaveTextContent('sonnet')
      expect(screen.getByText(/does not offer that/i)).toBeInTheDocument()

      // Nothing in the catalogue is ticked, and the default is not ticked either:
      // the chat HAS chosen, just not something this provider knows.
      fireEvent.click(modelTrigger())
      expect(ticked('Provider default')).toBe(false)
      expect(ticked('gpt-5.6-sol')).toBe(false)
    })

    it('says nothing of the sort when the selection IS in the catalogue', () => {
      setup({ model: 'gpt-5.6-luna', effort: 'max' })
      expect(screen.queryByText(/does not offer that/i)).toBeNull()
    })
  })

  describe("a provider whose default model declares no levels (codex's real shape)", () => {
    // codex's effort catalogue is per-model with NO fallback, so there is no ''
    // key: its own default model has no declared levels at all.
    const perModelOnly = provider({
      efforts: {
        'gpt-5.6-sol': ['low', 'medium', 'high', 'max', 'ultra'],
        'gpt-5.6-luna': ['low', 'medium', 'high', 'max'],
      },
    })

    it('draws no effort control at all until a model is chosen', () => {
      setup({ provider: perModelOnly })
      expect(modelTrigger()).toBeTruthy()
      expect(queryEffortTrigger()).toBeNull()
    })

    it('reveals that model\u2019s levels once it is picked', async () => {
      setupLive({ provider: perModelOnly })
      expect(queryEffortTrigger()).toBeNull()

      fireEvent.click(modelTrigger())
      fireEvent.click(item('gpt-5.6-sol'))

      await waitFor(() => expect(queryEffortTrigger()).not.toBeNull())
      fireEvent.click(effortTrigger())
      expect(item('ultra')).toBeTruthy()
    })
  })

  it('keeps the accepted choice once the chat owner has written it back', async () => {
    setupLive()

    fireEvent.click(modelTrigger())
    fireEvent.click(item('gpt-5.6-luna'))
    await waitFor(() => expect(modelTrigger()).toHaveTextContent('gpt-5.6-luna'))

    fireEvent.click(effortTrigger())
    fireEvent.click(item('max'))
    await waitFor(() => expect(effortTrigger()).toHaveTextContent('max'))
    expect(setSelectionFn).toHaveBeenLastCalledWith('w1', 'c1', 'gpt-5.6-luna', 'max')
  })

  it('says a switch RESUMES the conversation rather than losing it', () => {
    // The next message restarts the CLI into the same session. That is worth
    // saying and it is not destructive, so it must not be dressed as a warning.
    setup()
    const hint = modelTrigger().getAttribute('title') ?? ''
    expect(hint).toMatch(/next message/i)
    expect(hint).toMatch(/Codex/)
    expect(hint).toMatch(/resumes this conversation/i)
  })
})
