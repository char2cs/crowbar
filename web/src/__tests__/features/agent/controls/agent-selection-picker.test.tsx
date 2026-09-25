import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { AgentSelectionPicker } from '@/features/agent/controls/agent-selection-picker'

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

function openMenu() {
  fireEvent.click(screen.getByRole('button', { name: /Agent:/ }))
}

describe('AgentSelectionPicker', () => {
  it('renders nothing when no offered provider declares a model catalogue', () => {
    const noCatalogue: AgentProvider = { ...claude, modelSelect: false, models: [] }
    const { container } = render(
      <AgentSelectionPicker
        provider={noCatalogue}
        providers={[noCatalogue]}
        model=""
        effort=""
        onSelectionChange={vi.fn()}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the current provider icon, model and effort on the trigger', () => {
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        onSelectionChange={vi.fn()}
      />,
    )
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent('opus')
    expect(trigger).toHaveTextContent('High')
  })

  it('groups models under their own provider section, one section per offered provider', () => {
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        onSelectionChange={vi.fn()}
      />,
    )
    openMenu()
    expect(screen.getByText('Claude')).toBeInTheDocument()
    expect(screen.getByText('Codex')).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'sonnet' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'gpt-5.6-sol' })).toBeInTheDocument()
  })

  it('picking a model on the SAME provider stages provider, model and effort together', () => {
    const onSelectionChange = vi.fn()
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        onSelectionChange={onSelectionChange}
      />,
    )
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'sonnet' }))

    // sonnet declares high — the current effort survives the pick unchanged.
    expect(onSelectionChange).toHaveBeenCalledWith('claude', 'sonnet', 'high')
  })

  it('picking a model whose declared efforts do not include the current one falls back to its own first level', () => {
    const onSelectionChange = vi.fn()
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="xhigh"
        onSelectionChange={onSelectionChange}
      />,
    )
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'sonnet' }))

    // sonnet has no xhigh — lands on sonnet's own first declared level.
    expect(onSelectionChange).toHaveBeenCalledWith('claude', 'sonnet', 'low')
  })

  // THE REGRESSION this session fixed. The picker used to call a LIVE
  // onSwitchProvider (killing the outgoing CLI and spawning the new one) the
  // instant a cross-provider row was clicked — before the user ever sent
  // anything. The picker now has no such prop at all: a cross-provider pick
  // stages exactly like a same-provider one, and the switch (if any) happens
  // server-side, atomically with the next actual send.
  it('picking a model on a DIFFERENT provider only stages it — no live switch, nothing async', () => {
    const onSelectionChange = vi.fn()
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        onSelectionChange={onSelectionChange}
      />,
    )
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'gpt-5.6-sol' }))

    // gpt-5.6-sol declares high too — the current effort survives the pick
    // unchanged, same rule as a same-provider model pick.
    expect(onSelectionChange).toHaveBeenCalledWith('codex', 'gpt-5.6-sol', 'high')
  })

  it('typing a provider name surfaces every model under it', () => {
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        onSelectionChange={vi.fn()}
      />,
    )
    openMenu()
    fireEvent.change(screen.getByPlaceholderText(/Search models/), { target: { value: 'codex' } })

    expect(screen.getByText('Codex')).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'gpt-5.6-sol' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: 'gpt-5.6-luna' })).toBeInTheDocument()
    expect(screen.queryByText('Claude')).not.toBeInTheDocument()
  })

  it('filters to matching models only when the query names no provider', () => {
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        onSelectionChange={vi.fn()}
      />,
    )
    openMenu()
    fireEvent.change(screen.getByPlaceholderText(/Search models/), { target: { value: 'haiku' } })

    expect(screen.getByRole('menuitem', { name: 'haiku' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'sonnet' })).not.toBeInTheDocument()
    expect(screen.queryByText('Codex')).not.toBeInTheDocument()
  })

  it('renders the effort slider for the CURRENT model, not the whole provider', () => {
    render(
      <AgentSelectionPicker
        provider={codex}
        providers={[claude, codex]}
        model="gpt-5.6-sol"
        effort="high"
        onSelectionChange={vi.fn()}
      />,
    )
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    const effortSection = slider.parentElement?.parentElement as HTMLElement
    expect(within(effortSection).getByText('Ultra')).toBeInTheDocument()
  })

  // Regression: an unconfirmed effort ("Default", or any string absent from
  // the model's own levels) used to clamp to index 0 and draw the thumb
  // sitting on "Low" — a level nobody picked or the provider ever reported,
  // read live as the slider looking pre-filled when nothing was known.
  it('draws no thumb/fill for an effort the current model does not declare', () => {
    render(
      <AgentSelectionPicker
        provider={codex}
        providers={[claude, codex]}
        model="gpt-5.6-sol"
        effort="Default"
        onSelectionChange={vi.fn()}
      />,
    )
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    expect(slider).not.toHaveAttribute('aria-valuenow')
    expect(slider.querySelector('.bg-primary')).not.toBeInTheDocument()
    // The tick row and its labels still render — a real pick is still
    // possible, only nothing is highlighted as already chosen.
    expect(screen.getByRole('button', { name: 'Low' })).toBeInTheDocument()
  })

  // Regression: the first cut of this control only responded to a click on
  // the track or a tick label underneath it — it LOOKED like a slider but
  // could not be dragged, which is not a slider. Pressing and moving across
  // the track must track continuously, not just jump once per click.
  //
  // Tracking is done with plain `window` pointermove/pointerup listeners,
  // NOT the element Pointer Capture API — confirmed live against the real
  // desktop app's WKWebView that `setPointerCapture` throws NotFoundError
  // even for a genuine user pointerdown, which silently ate the press-to-
  // jump too (it aborted the handler before the value was ever set). So
  // this fires move/up on `window`, matching where the component actually
  // listens, and never stubs Pointer Capture at all — a capture-throwing
  // environment is exactly what this must keep working under.
  it('is an actually-draggable slider, not just a click-to-jump bar', () => {
    const onSelectionChange = vi.fn()
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="low"
        onSelectionChange={onSelectionChange}
      />,
    )
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
    render(
      <AgentSelectionPicker
        provider={codex}
        providers={[claude, codex]}
        model="gpt-5.6-luna"
        effort=""
        onSelectionChange={vi.fn()}
      />,
    )
    openMenu()
    expect(screen.queryByRole('slider')).not.toBeInTheDocument()
  })

  it('clicking an effort tick changes only the effort, keeping the model', () => {
    const onSelectionChange = vi.fn()
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        onSelectionChange={onSelectionChange}
      />,
    )
    openMenu()
    fireEvent.click(screen.getByRole('button', { name: 'Max' }))

    expect(onSelectionChange).toHaveBeenCalledWith('claude', 'opus', 'max')
  })

  it('is a pure local pick — never calls any network API itself', () => {
    // No wsId/chatId prop exists on this component at all, and no
    // onSwitchProvider prop either: the type surface itself is the
    // regression guard against a committing call — or a live provider
    // switch — creeping back in. This test documents that intent for a
    // reader who only sees the rendered behavior.
    const onSelectionChange = vi.fn()
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        onSelectionChange={onSelectionChange}
      />,
    )
    openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: 'sonnet' }))
    expect(onSelectionChange).toHaveBeenCalledTimes(1)
  })

  it('marks the current model as picked (a checkmark) and no other row', () => {
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        onSelectionChange={vi.fn()}
      />,
    )
    openMenu()
    const opusRow = screen.getByRole('menuitem', { name: 'opus' })
    const sonnetRow = screen.getByRole('menuitem', { name: 'sonnet' })
    expect(opusRow.querySelector('svg')).toBeTruthy()
    expect(sonnetRow.querySelector('svg')).toBeNull()
  })

  it('shows nothing selected when model/effort are unset — the "unfired hook" state, not a bug', () => {
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model=""
        effort=""
        onSelectionChange={vi.fn()}
      />,
    )
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveAccessibleName('Agent: Claude, model unset, effort unset')
    openMenu()
    for (const name of ['sonnet', 'opus', 'haiku']) {
      expect(screen.getByRole('menuitem', { name }).querySelector('svg')).toBeNull()
    }
  })

  // agent-api.ts's own doc on `AgentProvider.efforts`: "Read it as
  // efforts[model] where model is '' when the chat has no selection" — a
  // real, backend-resolved entry, not a gap. `effortLevelsFor` does exactly
  // that lookup with no special-casing of '' at all, so a caller handing the
  // picker the RAW unset value ('') finds it and draws the slider, unlike a
  // label ("Default") baked into the value instead — see AgentChatView's own
  // `model`/`effectiveEffort`, which is the whole bug this test file's
  // sibling (agent-chat-view.test.tsx) regression-tests.
  it("finds the provider's own '' entry and draws the slider when no model is confirmed yet", () => {
    const withDefaultEfforts: AgentProvider = {
      ...claude,
      efforts: { ...claude.efforts, '': ['low', 'medium'] },
    }
    render(
      <AgentSelectionPicker
        provider={withDefaultEfforts}
        providers={[withDefaultEfforts, codex]}
        model=""
        effort=""
        onSelectionChange={vi.fn()}
      />,
    )
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    const effortSection = slider.parentElement?.parentElement as HTMLElement
    expect(within(effortSection).getByText('Low')).toBeInTheDocument()
    expect(within(effortSection).getByText('Medium')).toBeInTheDocument()
  })

  // The mirror case: a provider with NO '' entry (every fixture above except
  // the one just added) offers no effort levels at all until a real model is
  // picked — absence, not a disabled slider, same house rule as the whole
  // control follows for a provider with no catalogue.
  it('renders no slider for an unconfirmed model when the provider declares no own-default entry', () => {
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model=""
        effort=""
        onSelectionChange={vi.fn()}
      />,
    )
    openMenu()
    expect(screen.queryByRole('slider')).not.toBeInTheDocument()
  })

  // The user-reported bug: a completed turn leaves the trigger reading
  // "Default" forever, with no clue which model actually answered, even
  // though the provider told Crowbar. `reportedModel` is display-only — it
  // must show up on the trigger without becoming the SELECTION.
  it('shows the provider-reported model next to the unset label when nothing is explicitly picked', () => {
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model=""
        effort=""
        reportedModel="Claude Sonnet 4.5"
        onSelectionChange={vi.fn()}
      />,
    )
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent('unset')
    expect(trigger).toHaveTextContent('(Claude Sonnet 4.5)')
    expect(trigger).toHaveAccessibleName(
      'Agent: Claude, model unset (reported Claude Sonnet 4.5), effort unset',
    )
  })

  it('an explicit model selection ignores a reported model entirely', () => {
    render(
      <AgentSelectionPicker
        provider={claude}
        providers={[claude, codex]}
        model="opus"
        effort="high"
        reportedModel="Claude Sonnet 4.5"
        onSelectionChange={vi.fn()}
      />,
    )
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveTextContent('opus')
    expect(trigger).not.toHaveTextContent('Sonnet')
    expect(trigger).toHaveAccessibleName('Agent: Claude, model opus, effort high')
  })

  // THE REGRESSION this area already broke once: a label baked into `model`
  // stopped `efforts['']` from resolving. `reportedModel` must not repeat
  // that mistake — the slider still has to find the provider's own ''
  // entry, and a pick still has to stage the RAW '' model, never the report.
  it("a reported model does not break the unset model's own efforts[''] slider or leak into a pick", () => {
    const withDefaultEfforts: AgentProvider = {
      ...claude,
      efforts: { ...claude.efforts, '': ['low', 'medium'] },
    }
    const onSelectionChange = vi.fn()
    render(
      <AgentSelectionPicker
        provider={withDefaultEfforts}
        providers={[withDefaultEfforts, codex]}
        model=""
        effort=""
        reportedModel="Claude Sonnet 4.5"
        onSelectionChange={onSelectionChange}
      />,
    )
    openMenu()
    const slider = screen.getByRole('slider', { name: /Reasoning effort/ })
    const effortSection = slider.parentElement?.parentElement as HTMLElement
    expect(within(effortSection).getByText('Low')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Medium' }))
    expect(onSelectionChange).toHaveBeenCalledWith('claude', '', 'medium')
  })
})
