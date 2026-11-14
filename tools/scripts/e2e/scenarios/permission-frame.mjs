import { askHook, createWorkspaceChat, hook } from '../lib/fixture.mjs'
import { focusRecent, openChat } from '../lib/ui.mjs'
import { assert } from '../lib/wait.mjs'

// A permission prompt that opens while its chat's view is hidden is on the
// chat's feed, so showing the chat shows its card.

const cardShown = (ws) => {
  const slot = document.querySelector(`[data-workspace-slot="${ws}"]`)
  return (
    !!slot &&
    slot.dataset.active === 'true' &&
    !!document.querySelector('[data-testid="agent-choice-prompt"]')
  )
}
const hiddenCard = (ws) => {
  const slot = document.querySelector(`[data-workspace-slot="${ws}"]`)
  return !!slot && slot.dataset.active !== 'true'
}

export default {
  name: 'permission-frame',
  async run({ d, daemon, fx }) {
    const hidden = await createWorkspaceChat(daemon, fx)
    const front = await createWorkspaceChat(daemon, fx)
    await openChat(d, hidden.id)
    await openChat(d, front.id)
    await d.until('the first chat to be hidden behind the second', hiddenCard, hidden.workspaceId)

    await hook(daemon, fx, hidden, 'session_start', { session_id: 's-perm' })
    await hook(daemon, fx, hidden, 'user_prompt', {
      session_id: 's-perm',
      prompt: 'touch a file',
    })
    askHook(daemon, fx, hidden, 'permission', {
      session_id: 's-perm',
      prompt_id: 'p1',
      tool_name: 'Bash',
      tool_input: { command: 'touch e2e-file' },
    })
    assert(await focusRecent(d, hidden.id), 'the hidden chat has a Recents row')
    await d
      .until('the permission card on the first shown frame', cardShown, hidden.workspaceId, {
        timeout: 8000,
      })
      .catch(async (err) => {
        const seen = await d.eval(
          (ws) => ({
            active: document.querySelector('[data-workspace-slot][data-active="true"]')?.dataset
              .workspaceSlot,
            want: ws,
            cards: document.querySelectorAll('[data-testid="agent-choice-prompt"]').length,
            composer: document.body.textContent.slice(-300),
          }),
          hidden.workspaceId,
        )
        throw new Error(`${err.message}: ${JSON.stringify(seen)}`)
      })
  },
}
