import { createWorkspaceChat, hook } from '../lib/fixture.mjs'
import { openChat } from '../lib/ui.mjs'
import { assert } from '../lib/wait.mjs'

// A provider that records its message only when the turn closes streams text
// before a tool runs, then more text after it. The text bubble must stay above
// the tool row it preceded, not sink below it when the message grows.

const order = () => {
  const rows = [...document.querySelectorAll('[data-testid="agent-message-list"] [data-index]')]
  const position = (needle) => rows.findIndex((row) => row.textContent.includes(needle))
  return {
    message: position('BEFORE-TOOL'),
    tool: position('ORDER-CHECK'),
    after: rows.findIndex((r) => r.textContent.includes('AFTER-TOOL')),
  }
}

export default {
  name: 'codex-order',
  async run({ d, daemon, fx }) {
    const chat = await createWorkspaceChat(daemon, fx)
    await openChat(d, chat.id)
    const base = {
      session_id: 's-order',
      turn_id: 't-order',
      message_id: 'm-order',
      final: false,
    }
    await hook(daemon, fx, chat, 'session_start', { session_id: 's-order' })
    await hook(daemon, fx, chat, 'user_prompt', {
      session_id: 's-order',
      prompt: 'speak, work, speak',
    })
    await hook(daemon, fx, chat, 'message_delta', {
      ...base,
      index: 0,
      delta: 'BEFORE-TOOL ',
    })
    await d.until('the streamed text', () => document.body.textContent.includes('BEFORE-TOOL'))
    await hook(daemon, fx, chat, 'tool_pre', {
      session_id: 's-order',
      tool_use_id: 'tool-order',
      tool_name: 'Bash',
      tool_input: { command: 'ORDER-CHECK' },
    })
    await d.until('the tool row', () => document.body.textContent.includes('ORDER-CHECK'))
    await hook(daemon, fx, chat, 'message_delta', {
      ...base,
      index: 1,
      delta: 'AFTER-TOOL',
    })
    await d.until('the grown text', () => document.body.textContent.includes('AFTER-TOOL'))

    const rows = await d.eval(order)
    assert(rows.message >= 0 && rows.tool >= 0, `both rows rendered (${JSON.stringify(rows)})`)
    assert(
      rows.message < rows.tool,
      `the bubble must stay above the tool row (${JSON.stringify(rows)})`,
    )
  },
}
