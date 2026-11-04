import { beforeEach, describe, expect, it } from 'vitest'
import { domAgentBody, type DomOp } from '@/features/browser/lib/dom-agent'

const AsyncFunction = Object.getPrototypeOf(async () => {}).constructor as new (
  body: string,
) => () => Promise<unknown>

/** Runs one op the way the app does: as an async function body in the page. */
function run(op: DomOp, args: Record<string, unknown> = {}): Promise<unknown> {
  return new AsyncFunction(domAgentBody(op, args))()
}

function refOf(snapshot: unknown, needle: string): string {
  const line = String(snapshot)
    .split('\n')
    .find((l) => l.includes(needle))
  const ref = line?.match(/\[(e\d+)\]/)?.[1]
  if (!ref) throw new Error(`no ref for ${needle} in:\n${String(snapshot)}`)
  return ref
}

beforeEach(() => {
  document.body.innerHTML = ''
  document.title = 'Test page'
})

describe('snapshot', () => {
  it('lists roles, names, values and links, with refs on what can be acted on', async () => {
    document.body.innerHTML = `
      <h1>Welcome</h1>
      <a href="https://example.com/next">Next page</a>
      <label for="q">Search</label><input id="q" type="text" value="cats">
      <input type="checkbox" aria-label="Agree" checked>
      <input type="password" aria-label="Secret" value="hunter2">
      <button disabled>Save</button>
      <p>Some visible text</p>`
    const out = String(await run('snapshot'))
    expect(out).toContain('Page: Test page')
    expect(out).toContain('- heading "Welcome" level=1')
    expect(out).toMatch(/- link "Next page" \[e\d+\] href="https:\/\/example.com\/next"/)
    expect(out).toMatch(/- textbox "Search" \[e\d+\] value="cats"/)
    expect(out).toMatch(/- checkbox "Agree" \[e\d+\] checked/)
    expect(out).toMatch(/- button "Save" \[e\d+\] disabled/)
    expect(out).toContain('- text: "Some visible text"')
    expect(out).not.toContain('hunter2')
  })

  it('leaves hidden content out', async () => {
    document.body.innerHTML = `
      <button style="display:none">Ghost</button>
      <div aria-hidden="true"><button>Aria ghost</button></div>
      <script>var x = 1</script><button>Real</button>`
    const out = String(await run('snapshot'))
    expect(out).toContain('Real')
    expect(out).not.toContain('Ghost')
    expect(out).not.toContain('var x')
  })

  it('keeps an element on the same ref across snapshots', async () => {
    document.body.innerHTML = '<button>One</button><button>Two</button>'
    const first = await run('snapshot')
    document.body.insertAdjacentHTML('afterbegin', '<button>Zero</button>')
    const second = await run('snapshot')
    expect(refOf(second, '"Two"')).toBe(refOf(first, '"Two"'))
  })

  it('is bounded and says so', async () => {
    document.body.innerHTML = Array.from({ length: 400 }, (_, i) => `<p>row ${i}</p>`).join('')
    const out = String(await run('snapshot', { maxChars: 1000 }))
    expect(out.length).toBeLessThan(1400)
    expect(out).toMatch(/\.\.\. \[truncated: \d+ more lines not shown\]/)
  })
})

describe('actions', () => {
  it('clicks the element a ref names', async () => {
    document.body.innerHTML = '<button id="b">Go</button>'
    let clicks = 0
    document.getElementById('b')?.addEventListener('click', () => clicks++)
    const ref = refOf(await run('snapshot'), '"Go"')
    expect(await run('click', { ref })).toBe('clicked button "Go"')
    expect(clicks).toBe(1)
  })

  it('refuses a ref that left the page and asks for a new snapshot', async () => {
    document.body.innerHTML = '<button id="b">Go</button>'
    const ref = refOf(await run('snapshot'), '"Go"')
    document.body.innerHTML = ''
    await expect(run('click', { ref })).rejects.toThrow(/take a new snapshot/)
    await expect(run('click', { ref: 'e999' })).rejects.toThrow(/take a new snapshot/)
  })

  it('types into a field, fires input and change, and can submit its form', async () => {
    document.body.innerHTML = '<form><input id="q" aria-label="Query"></form>'
    const events: string[] = []
    const input = document.getElementById('q') as HTMLInputElement
    input.addEventListener('input', () => events.push(`input:${input.value}`))
    input.addEventListener('change', () => events.push('change'))
    let submitted = false
    document.querySelector('form')?.addEventListener('submit', (e) => {
      e.preventDefault()
      submitted = true
    })
    const ref = refOf(await run('snapshot'), '"Query"')

    await run('type', { ref, text: 'hello', submit: true })
    expect(input.value).toBe('hello')
    expect(events).toEqual(['input:hello', 'change'])
    expect(submitted).toBe(true)
  })

  it('selects an option by its text and sends checkboxes to click', async () => {
    document.body.innerHTML =
      '<select aria-label="Size"><option value="s">Small</option><option value="l">Large</option></select><input type="checkbox" aria-label="Opt">'
    const snap = await run('snapshot')
    await run('type', { ref: refOf(snap, '"Size"'), text: 'Large' })
    expect((document.querySelector('select') as HTMLSelectElement).value).toBe('l')
    await expect(run('type', { ref: refOf(snap, '"Opt"'), text: 'x' })).rejects.toThrow(
      /browser_click/,
    )
  })

  it('presses keys at the focused element', async () => {
    document.body.innerHTML = '<input id="a">'
    const input = document.getElementById('a') as HTMLInputElement
    input.focus()
    const keys: string[] = []
    input.addEventListener('keydown', (e) => keys.push(e.key))
    await run('press_key', { key: 'Escape' })
    expect(keys).toEqual(['Escape'])
  })

  it('scrolls an element into view or the page by direction', async () => {
    document.body.innerHTML = '<button>Far</button>'
    let scrolled = 0
    document.querySelector('button')!.scrollIntoView = () => void scrolled++
    window.scrollTo = () => {}
    const ref = refOf(await run('snapshot'), '"Far"')
    expect(String(await run('scroll', { ref }))).toMatch(/^scrolled to/)
    expect(scrolled).toBe(1)
    expect(String(await run('scroll', { direction: 'bottom' }))).toMatch(/^scrolled to/)
  })
})

describe('wait', () => {
  it('resolves at once when the condition already holds', async () => {
    document.body.innerHTML = '<p id="x">hi there</p>'
    await expect(run('wait', { condition: 'selector', value: '#x' })).resolves.toContain('already')
    await expect(run('wait', { condition: 'text', value: 'hi there' })).resolves.toContain(
      'already',
    )
  })

  it('resolves when a mutation makes the selector appear', async () => {
    const waiting = run('wait', { condition: 'selector', value: '#late', timeoutMs: 5000 })
    queueMicrotask(() => document.body.insertAdjacentHTML('beforeend', '<i id="late"></i>'))
    await expect(waiting).resolves.toContain('satisfied')
  })

  it('fails at its deadline and rejects unknown conditions', async () => {
    await expect(run('wait', { condition: 'text', value: 'never', timeoutMs: 1 })).rejects.toThrow(
      /timed out/,
    )
    await expect(run('wait', { condition: 'vibes' })).rejects.toThrow(/unknown wait condition/)
  })
})

it('rejects an unknown op', async () => {
  await expect(run('nope' as DomOp)).rejects.toThrow(/unknown browser op/)
})
