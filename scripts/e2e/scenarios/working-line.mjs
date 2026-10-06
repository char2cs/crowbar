import { createWorkspaceChat, hook } from '../lib/fixture.mjs'
import { openChat } from '../lib/ui.mjs'
import { assert, assertEqual } from '../lib/wait.mjs'

// The working line's heading is the agent's own reasoning headline when it is
// telling us, else Crowbar's rotating verb; it wears the heading face.

const heading = () => {
  const el = document.querySelector('[data-testid="agent-working-heading"]')
  if (!el) return null
  const style = getComputedStyle(el)
  const token = getComputedStyle(document.documentElement).getPropertyValue('--font-heading').trim()
  return {
    text: el.textContent,
    family: style.fontFamily,
    weight: style.fontWeight,
    token,
  }
}

export default {
  name: 'working-line',
  async run({ d, daemon, fx }) {
    const chat = await createWorkspaceChat(daemon, fx)
    await openChat(d, chat.id)
    const send = (event, payload) =>
      hook(daemon, fx, chat, event, { session_id: 's-work', ...payload })
    await send('session_start', {})
    await send('user_prompt', { prompt: 'think' })

    // Nothing said yet: the heading is a verb with an ellipsis.
    const verb = await d.until('the working heading', heading)
    assert(verb.text.endsWith('…'), `without reasoning the heading is a verb (got "${verb.text}")`)

    await send('reasoning_delta', {
      turn_id: 't-work',
      message_id: 'r1',
      index: 0,
      final: false,
      delta: '**Weighing the options**\nthe long body of the thought',
    })
    const thought = await d.until('the reasoning headline', () => {
      const h = document.querySelector('[data-testid="agent-working-heading"]')
      return h && h.textContent === 'Weighing the options' ? h.textContent : null
    })
    assertEqual(thought, 'Weighing the options', 'the heading shows the agent reasoning headline')
    assert(
      await d.eval(() =>
        document
          .querySelector('[data-testid="agent-reasoning"]')
          ?.textContent.includes('long body'),
      ),
      'the body sits under the heading',
    )

    // The heading wears the heading face at regular weight (the face is single-weight: no faux bold).
    const style = await d.eval(heading)
    const face = style.token.split(',')[0].replace(/["']/g, '').trim()
    assert(
      style.family.includes(face),
      `heading font-family ${style.family} should lead with the --font-heading token ${style.token}`,
    )
    assertEqual(style.weight, '400', 'the heading is not faux-bolded')
  },
}
