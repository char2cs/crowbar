/* global clearTimeout, document, Event, getComputedStyle, KeyboardEvent, location, MouseEvent, MutationObserver, Node, setTimeout, window -- runs in the page, not the app */
// Injected into an agent-driven page as `const run = (<this file>)()`; `run(op, args)`
// does one DOM operation. It is plain page script: nothing here can reach the app.
// Bump VERSION when an op's output shape changes.
// eslint-disable-next-line @typescript-eslint/no-unused-vars -- the host evaluates this file as text, so nothing here references the name
function pageAgent() {
  const VERSION = 1
  const STATE_KEY = Symbol.for('crowbar.browserAgent')
  const state = (window[STATE_KEY] ??= { next: 1, ids: new WeakMap(), refs: new Map() })

  const SKIPPED = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'HEAD', 'META', 'LINK'])
  const TAG_ROLES = {
    A: 'link',
    BUTTON: 'button',
    SELECT: 'combobox',
    TEXTAREA: 'textbox',
    IMG: 'img',
    UL: 'list',
    OL: 'list',
    LI: 'listitem',
    NAV: 'navigation',
    MAIN: 'main',
    HEADER: 'banner',
    FOOTER: 'contentinfo',
    FORM: 'form',
    TABLE: 'table',
    DIALOG: 'dialog',
    SUMMARY: 'button',
  }
  const INPUT_ROLES = {
    button: 'button',
    submit: 'button',
    reset: 'button',
    image: 'button',
    checkbox: 'checkbox',
    radio: 'radio',
    range: 'slider',
    search: 'searchbox',
  }
  const ACTIONABLE = new Set([
    'link',
    'button',
    'textbox',
    'searchbox',
    'checkbox',
    'radio',
    'combobox',
    'slider',
    'switch',
    'tab',
    'menuitem',
    'option',
  ])

  const clean = (text, max = 160) => {
    const flat = (text ?? '').replace(/\s+/g, ' ').trim()
    return flat.length > max ? `${flat.slice(0, max)}...` : flat
  }

  function roleOf(el) {
    const explicit = el.getAttribute('role')
    if (explicit) return explicit.split(' ')[0]
    if (el.tagName === 'INPUT') return INPUT_ROLES[(el.type || 'text').toLowerCase()] ?? 'textbox'
    if (/^H[1-6]$/.test(el.tagName)) return 'heading'
    if (el.tagName === 'A' && !el.hasAttribute('href')) return null
    if (el.isContentEditable && el.getAttribute('contenteditable') !== null) return 'textbox'
    return TAG_ROLES[el.tagName] ?? null
  }

  function nameOf(el) {
    const label = el.getAttribute('aria-label')
    if (label) return clean(label)
    const by = el.getAttribute('aria-labelledby')
    if (by) {
      const text = by
        .split(' ')
        .map((id) => document.getElementById(id)?.textContent ?? '')
        .join(' ')
      if (clean(text)) return clean(text)
    }
    if (el.labels?.length) return clean(el.labels[0].textContent)
    if (el.tagName === 'IMG') return clean(el.getAttribute('alt'))
    if (el.tagName === 'INPUT' && ['button', 'submit', 'reset'].includes(el.type)) {
      return clean(el.value)
    }
    if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
      return clean(el.getAttribute('placeholder') ?? el.getAttribute('title'))
    }
    return clean(el.textContent) || clean(el.getAttribute('title'))
  }

  function hidden(el) {
    if (el.hidden || el.getAttribute('aria-hidden') === 'true') return true
    const style = getComputedStyle(el)
    return style.display === 'none' || style.visibility === 'hidden'
  }

  function refOf(el) {
    let id = state.ids.get(el)
    if (!id) {
      id = `e${state.next++}`
      state.ids.set(el, id)
      state.refs.set(id, new WeakRef(el))
    }
    return id
  }

  function resolve(ref) {
    const el = state.refs.get(String(ref).replace(/^\[|\]$/g, ''))?.deref()
    if (!el || !el.isConnected) {
      throw new Error(`ref ${ref} is gone from the page; take a new snapshot`)
    }
    return el
  }

  function describe(el, role) {
    const parts = [`- ${role}`]
    const name = nameOf(el)
    if (name) parts.push(JSON.stringify(name))
    if (/^H[1-6]$/.test(el.tagName)) parts.push(`level=${el.tagName[1]}`)
    if (ACTIONABLE.has(role)) parts.push(`[${refOf(el)}]`)
    const plain = !['password', 'checkbox', 'radio'].includes(el.type)
    if ('value' in el && plain && el.value && role !== 'button') {
      parts.push(`value=${JSON.stringify(clean(el.value, 120))}`)
    }
    if (el.checked) parts.push('checked')
    if (el.disabled) parts.push('disabled')
    if (el.tagName === 'A' && el.href) parts.push(`href=${JSON.stringify(el.href)}`)
    return parts.join(' ')
  }

  function snapshot({ maxChars = 20000 } = {}) {
    for (const [id, weak] of state.refs) if (!weak.deref()?.isConnected) state.refs.delete(id)
    const limit = Math.min(Math.max(Number(maxChars) || 20000, 1000), 100000)
    const lines = [`Page: ${document.title || '(untitled)'} (${location.href})`]
    let used = lines[0].length
    let omitted = 0

    const push = (line) => {
      if (used + line.length > limit) {
        omitted++
        return
      }
      used += line.length + 1
      lines.push(line)
    }

    const walk = (node, depth) => {
      for (const child of node.childNodes) {
        if (child.nodeType === Node.TEXT_NODE) {
          const text = clean(child.textContent, 200)
          if (text) push(`${'  '.repeat(depth)}- text: ${JSON.stringify(text)}`)
          continue
        }
        if (child.nodeType !== Node.ELEMENT_NODE) continue
        if (SKIPPED.has(child.tagName) || hidden(child)) continue
        if (child.tagName === 'LABEL' && child.control) continue
        const role = roleOf(child)
        if (role) push('  '.repeat(depth) + describe(child, role))
        const leaf = role === 'button' || role === 'link' || role === 'heading' || role === 'img'
        if (!leaf) walk(child.shadowRoot ?? child, role ? depth + 1 : depth)
      }
    }
    walk(document.body ?? document.documentElement, 0)
    if (omitted > 0) lines.push(`... [truncated: ${omitted} more lines not shown]`)
    return lines.join('\n')
  }

  function describeTarget(el) {
    const name = nameOf(el)
    return `${roleOf(el) ?? el.tagName.toLowerCase()}${name ? ` ${JSON.stringify(name)}` : ''}`
  }

  function click({ ref }) {
    const el = resolve(ref)
    el.scrollIntoView({ block: 'center', inline: 'center' })
    el.focus?.()
    const init = { bubbles: true, cancelable: true }
    el.dispatchEvent(new MouseEvent('mousedown', init))
    el.dispatchEvent(new MouseEvent('mouseup', init))
    el.click()
    return `clicked ${describeTarget(el)}`
  }

  function setValue(el, text) {
    const proto = Object.getPrototypeOf(el)
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set
    if (setter) setter.call(el, text)
    else el.value = text
  }

  function pressKey(target, key) {
    const init = { key, bubbles: true, cancelable: true }
    const down = target.dispatchEvent(new KeyboardEvent('keydown', init))
    if (key.length === 1 || key === 'Enter')
      target.dispatchEvent(new KeyboardEvent('keypress', init))
    target.dispatchEvent(new KeyboardEvent('keyup', init))
    // Untrusted key events run no browser default, so the common ones are done here.
    if (!down) return
    if (key === 'Enter') {
      if (target.form && target.tagName !== 'TEXTAREA') target.form.requestSubmit()
      else if (target.tagName === 'BUTTON' || target.tagName === 'A') target.click()
    } else if (key === ' ' && target.tagName === 'BUTTON') {
      target.click()
    } else if (key === 'Tab') {
      const order = [
        ...document.querySelectorAll('a[href],button,input,select,textarea,[tabindex]'),
      ].filter((el) => !el.disabled && el.tabIndex >= 0 && !hidden(el))
      const next = order[(order.indexOf(target) + 1) % Math.max(order.length, 1)]
      next?.focus()
    }
  }

  function type({ ref, text, submit }) {
    const el = resolve(ref)
    if (el.tagName === 'INPUT' && ['checkbox', 'radio'].includes(el.type)) {
      throw new Error('this is a checkbox or radio; use browser_click')
    }
    el.scrollIntoView({ block: 'center' })
    el.focus()
    if (el.tagName === 'SELECT') {
      const option = [...el.options].find((o) => o.value === text || o.text.trim() === text)
      if (!option) throw new Error(`no option ${JSON.stringify(text)} in this select`)
      el.value = option.value
    } else if (el.isContentEditable) {
      el.textContent = text
    } else {
      setValue(el, text)
    }
    el.dispatchEvent(new Event('input', { bubbles: true }))
    el.dispatchEvent(new Event('change', { bubbles: true }))
    if (submit) pressKey(el, 'Enter')
    return `typed into ${describeTarget(el)}${submit ? ' and pressed Enter' : ''}`
  }

  function press_key({ key }) {
    pressKey(document.activeElement ?? document.body, key)
    return `pressed ${key}`
  }

  function scroll({ ref, direction = 'down' }) {
    if (ref) {
      resolve(ref).scrollIntoView({ block: 'center' })
    } else if (direction === 'top') {
      window.scrollTo(0, 0)
    } else if (direction === 'bottom') {
      window.scrollTo(0, document.documentElement.scrollHeight)
    } else {
      window.scrollBy(0, (direction === 'up' ? -1 : 1) * window.innerHeight * 0.8)
    }
    const { scrollHeight } = document.documentElement
    return `scrolled to ${Math.round(window.scrollY)} of ${scrollHeight}`
  }

  function wait({ condition, value = '', timeoutMs = 10000 }) {
    const checks = {
      load: () => document.readyState === 'complete',
      selector: () => document.querySelector(value) !== null,
      text: () => (document.body?.innerText ?? document.body?.textContent ?? '').includes(value),
    }
    const check = checks[condition]
    if (!check) throw new Error(`unknown wait condition ${JSON.stringify(condition)}`)
    return new Promise((resolveWait, reject) => {
      if (check()) return resolveWait(`${condition} is already satisfied`)
      const done = (fn, arg) => {
        observer.disconnect()
        window.removeEventListener('load', recheck)
        clearTimeout(deadline)
        fn(arg)
      }
      const recheck = () => check() && done(resolveWait, `${condition} satisfied`)
      const observer = new MutationObserver(recheck)
      observer.observe(document.documentElement, {
        subtree: true,
        childList: true,
        attributes: true,
        characterData: true,
      })
      window.addEventListener('load', recheck)
      const deadline = setTimeout(
        () =>
          done(
            reject,
            new Error(`timed out after ${timeoutMs}ms waiting for ${condition} ${value}`.trim()),
          ),
        timeoutMs,
      )
    })
  }

  const ops = { snapshot, click, type, press_key, scroll, wait }
  const run = (op, args) => {
    const fn = ops[op]
    if (!fn) throw new Error(`unknown browser op ${JSON.stringify(op)} (agent script v${VERSION})`)
    return fn(args ?? {})
  }
  // The pointer script builds on these, so refs from both come from one registry.
  run.helpers = { roleOf, nameOf, hidden, refOf }
  return run
}
