import { createWorkspaceChat, hook, hookAck } from '../lib/fixture.mjs'
import { focusRecent, openChat } from '../lib/ui.mjs'
import { assert, until } from '../lib/wait.mjs'

// A permission prompt that opens while its chat's view is hidden rides the chat
// feed as a `choice` frame carrying the whole set, so the first frame the chat
// shows already has the card and needs no /activity read of its own.

// Withholds every /activity read of the chat (the request never settles), so a
// card that appears anyway cannot have come from one.
const withholdActivity = (chatId) => {
  if (!window.__e2eWrapped) {
    window.__e2eWrapped = true
    const original = window.fetch
    window.fetch = function (input, init) {
      const url = String(typeof input === 'string' ? input : (input.url ?? input))
      if (window.__e2eWithheld && url.includes(`/chats/${window.__e2eWithheld}/activity`)) {
        window.__e2eBlocked = (window.__e2eBlocked ?? 0) + 1
        return { then() {} }
      }
      return original.call(this, input, init)
    }
  }
  window.__e2eWithheld = chatId
  return true
}
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
    const ack = await hookAck(daemon, fx, hidden, 'permission', {
      session_id: 's-perm',
      prompt_id: 'p1',
      tool_name: 'Bash',
      tool_input: { command: 'touch e2e-file' },
    })
    assert(ack.await, 'the permission hook opened an awaited choice')
    await until('the daemon to hold one pending choice', async () => {
      const choices = await daemon.get(`${fx.base}/chats/${hidden.id}/choices`)
      return choices.length === 1
    })

    await d.eval(withholdActivity, hidden.id)
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
    // The withholding was live: the chat did try to read /activity, and still showed its card.
    await d.eval(() => {
      window.__e2eWithheld = null
      return true
    })
  },
}
