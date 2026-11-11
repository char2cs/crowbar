import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentComposer } from '@/features/agent/composer/agent-composer'
import type { ChatMarkdownEditorHandle } from '@/features/agent/composer/plate/chat-markdown-editor'

function draw(overrides: Partial<Parameters<typeof AgentComposer>[0]> = {}) {
  return render(
    <AgentComposer
      wsId="w1"
      chatId="c1"
      asks={[]}
      providerLabel="Claude"
      live
      working={false}
      compacting={false}
      sending={false}
      submitUnavailable={false}
      canStop={false}
      placement="dock"
      editorRef={createRef<ChatMarkdownEditorHandle>()}
      slashOpen={false}
      onShapeChange={vi.fn()}
      onKeyDown={vi.fn()}
      onSend={vi.fn()}
      onStop={vi.fn()}
      onOpenTerminal={vi.fn()}
      {...overrides}
    />,
  )
}

describe('AgentComposer pill compaction rain', () => {
  it('rains behind the pill while compacting, leaving the input usable', () => {
    const { container } = draw({ compacting: true })

    expect(container.querySelector('.pill svg.rain')).not.toBeNull()
    expect(screen.getByRole('textbox')).toBeVisible()
  })

  it('draws no rain otherwise', () => {
    const { container } = draw()

    expect(container.querySelector('svg.rain')).toBeNull()
  })
})
