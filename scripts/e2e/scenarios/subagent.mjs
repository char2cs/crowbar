import { createWorkspaceChat, hook } from '../lib/fixture.mjs'
import { openChat } from '../lib/ui.mjs'
import { assertEqual } from '../lib/wait.mjs'

// Subagent activity must not leak into the parent chat: its tool calls and
// prompts (hooks carrying an agent id) draw no rows, and a harness hand-back
// envelope is shown as the provider's own machinery, never as the user's bubble.

const transcript = () => ({
  tools: [...document.querySelectorAll('[data-testid="agent-activity-tool"]')].map(
    (e) => e.textContent,
  ),
  roles: [...document.querySelectorAll('article[data-role]')].map((a) => [
    a.dataset.role,
    a.textContent,
  ]),
  harnessLabels: document.querySelectorAll('[data-testid="message-harness-label"]').length,
})

export default {
  name: 'subagent',
  async run({ d, daemon, fx }) {
    const chat = await createWorkspaceChat(daemon, fx)
    await openChat(d, chat.id)
    const send = (event, payload) =>
      hook(daemon, fx, chat, event, { session_id: 's-sub', ...payload })

    await send('session_start', {})
    await send('user_prompt', { prompt: 'delegate this' })
    // Subagent traffic: its tool pair and a prompt fired inside it.
    await send('tool_pre', {
      tool_use_id: 'sub-1',
      tool_name: 'Bash',
      tool_input: { command: 'SUBAGENT-ONLY' },
      agent_id: 'agent-1',
    })
    await send('tool_post', {
      tool_use_id: 'sub-1',
      tool_name: 'Bash',
      agent_id: 'agent-1',
    })
    await send('user_prompt', {
      prompt: 'SUBAGENT-PROMPT',
      agent_id: 'agent-1',
    })
    // Positive control: the parent's own tool call must render, so the empty
    // result above cannot be a transcript that never rendered anything.
    await send('tool_pre', {
      tool_use_id: 'main-1',
      tool_name: 'Bash',
      tool_input: { command: 'PARENT-TOOL' },
    })
    await d.until('the parent tool row', () => document.body.textContent.includes('PARENT-TOOL'))
    const live = await d.eval(transcript)
    assertEqual(
      live.tools.filter((t) => t.includes('SUBAGENT-ONLY')).length,
      0,
      'a subagent tool call draws no row',
    )
    assertEqual(
      live.tools.filter((t) => t.includes('PARENT-TOOL')).length,
      1,
      "the parent's own tool call draws one row",
    )

    // The hand-back envelope a provider injects when a background subagent finishes.
    await send('tool_post', { tool_use_id: 'main-1', tool_name: 'Bash' })
    await send('turn_stop', { last_assistant_message: 'waiting' })
    await send('user_prompt', {
      prompt:
        '<task-notification>\n<status>completed</status>\nHAND-BACK-BODY\n</task-notification>',
    })
    await d.until(
      'the harness row',
      () => document.querySelectorAll('[data-testid="message-harness-label"]').length === 1,
    )

    const seen = await d.eval(transcript)
    const page = await d.eval(() => document.body.textContent)
    assertEqual(
      page.includes('SUBAGENT-ONLY') || page.includes('SUBAGENT-PROMPT'),
      false,
      'nothing a subagent did is on the page',
    )
    const bubbles = seen.roles.filter(([role]) => role === 'user').map(([, text]) => text)
    assertEqual(
      bubbles.some((t) => t.includes('SUBAGENT-PROMPT')),
      false,
      'a subagent prompt is not the user',
    )
    assertEqual(
      bubbles.some((t) => t.includes('task-notification') || t.includes('HAND-BACK')),
      false,
      'the hand-back is not a user bubble',
    )
    assertEqual(
      seen.roles.filter(([role]) => role === 'harness').length,
      1,
      'the hand-back is one harness row',
    )

    const activity = await daemon.get(`${fx.base}/chats/${chat.id}/activity`)
    assertEqual(
      activity.toolCalls.map((t) => t.target),
      ['PARENT-TOOL'],
      'the daemon recorded only the parent tool call',
    )
  },
}
