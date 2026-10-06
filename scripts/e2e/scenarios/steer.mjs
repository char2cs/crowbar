import { createWorkspaceChat, getChat, hookAck } from '../lib/fixture.mjs'
import { liveStubProcesses } from '../lib/stub-provider.mjs'
import { openChat } from '../lib/ui.mjs'
import { assertEqual, until } from '../lib/wait.mjs'

// A prompt sent while the agent is mid-turn is parked and delivered by the
// turn-end hook into the SAME process (the provider descriptor declares steer),
// instead of waiting and respawning the CLI with the text in argv.

const userBubbles = () =>
  [...document.querySelectorAll('article[data-role="user"]')].map((a) => a.textContent)

export default {
  name: 'steer',
  async run({ d, daemon, fx, inst }) {
    const chat = await createWorkspaceChat(daemon, fx)
    await openChat(d, chat.id)
    const processes = liveStubProcesses(inst.runDir)
    const send = (event, payload) =>
      hookAck(daemon, fx, chat, event, { session_id: 's-steer', ...payload })

    await send('session_start', {})
    await send('user_prompt', { prompt: 'first' })

    const res = await daemon.call('POST', `${fx.base}/chats/${chat.id}/prompts`, {
      text: 'also check the tests',
      clientRequestId: crypto.randomUUID(),
    })
    assertEqual(res.status, 200, 'a prompt mid-turn is accepted, not refused as busy')
    assertEqual(
      (await getChat(daemon, fx, chat.id)).liveRunnerId,
      chat.runnerId,
      'the CLI is not replaced',
    )
    assertEqual(liveStubProcesses(inst.runDir), processes, 'no process was started or stopped')

    const ack = await send('turn_stop', {
      last_assistant_message: 'first done',
    })
    assertEqual(
      JSON.parse(ack.reply),
      { decision: 'block', reason: 'STEERED: also check the tests' },
      'the turn-end hook answers with the parked message',
    )
    assertEqual(
      (await getChat(daemon, fx, chat.id)).liveRunnerId,
      chat.runnerId,
      'still the same runner after delivery',
    )
    assertEqual(liveStubProcesses(inst.runDir), processes, 'still the same process after delivery')

    await send('turn_stop', { last_assistant_message: 'tests checked' })
    await until('the steered message in the transcript, once', async () => {
      const bubbles = await d.eval(userBubbles)
      return bubbles.filter((t) => t.includes('also check the tests')).length === 1
    })
    const messages = await daemon.get(`${fx.base}/chats/${chat.id}/messages?limit=50`)
    assertEqual(
      messages.items.map((m) => `${m.role}:${m.text}`),
      [
        'user:first',
        'assistant:first done',
        'user:also check the tests',
        'assistant:tests checked',
      ],
      'the steered prompt lands once, between the reply it followed and the one it caused',
    )
  },
}
