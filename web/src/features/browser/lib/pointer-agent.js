/* global document, getComputedStyle, location, window -- runs in the page, not the app */
// Injected after page-agent.js as `(<this file>)(base)`: the pick overlay and the
// `resolve` op. A pick is a Promise the host awaits, so the page needs no channel
// to the app; `pagehide` ends it, so a navigation never leaves it hanging.
// eslint-disable-next-line @typescript-eslint/no-unused-vars -- the host evaluates this file as text, so nothing here references the name
function pointerAgent(base) {
  const { roleOf, nameOf, hidden, refOf } = base.helpers
  const PICK_KEY = Symbol.for('crowbar.pick')
  const MAX_ITEMS = 10
  const DRAG_PX = 4
  const INSIDE = 0.6
  const HEADING = 'h1,h2,h3,h4,h5,h6,[role=heading]'
  const SKIPPED = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'HEAD', 'META', 'LINK'])
  const VISUAL = new Set(['IMG', 'INPUT', 'SELECT', 'TEXTAREA', 'BUTTON', 'VIDEO', 'CANVAS', 'svg'])
  const STYLES = [
    'color',
    'background-color',
    'font-size',
    'font-weight',
    'display',
    'position',
    'width',
    'height',
    'margin',
    'padding',
    'border',
  ]
  const HTML_CAP = 2000

  const flat = (text, max) => (text ?? '').replace(/\s+/g, ' ').trim().slice(0, max)
  const box = (el) => {
    const r = el.getBoundingClientRect()
    return [Math.round(r.left), Math.round(r.top), Math.round(r.width), Math.round(r.height)]
  }
  // innerText keeps the break between cells; jsdom has none, so it falls back.
  const textOf = (el) => el.innerText ?? el.textContent
  const tagOf = (el) => el.tagName.toLowerCase()

  function attr(name, value) {
    return `[${name}="${value.replace(/["\\]/g, '\\$&')}"]`
  }

  function anchorOf(el) {
    const id = el.getAttribute('id')
    if (id) return /^[A-Za-z_][\w-]*$/.test(id) ? `#${id}` : attr('id', id)
    const testId = el.getAttribute('data-testid')
    if (testId) return attr('data-testid', testId)
    const label = el.getAttribute('aria-label')
    if (label && label.length <= 40) return tagOf(el) + attr('aria-label', label)
    return null
  }

  function segmentOf(el) {
    const classes = [...el.classList]
      .filter(
        (c) => /^[A-Za-z][\w-]{0,23}$/.test(c) && !/\d{3,}/.test(c) && !/^(css|sc|jsx)-/.test(c),
      )
      .slice(0, 2)
    let segment = tagOf(el) + classes.map((c) => `.${c}`).join('')
    const same = [...(el.parentElement?.children ?? [])].filter((c) => c.tagName === el.tagName)
    if (same.length > 1) segment += `:nth-of-type(${same.indexOf(el) + 1})`
    return segment
  }

  function isUnique(selector, el) {
    try {
      const found = document.querySelectorAll(selector)
      return found.length === 1 && found[0] === el
    } catch {
      return false
    }
  }

  function selectorOf(el) {
    const parts = []
    for (let node = el; node && node !== document.documentElement; node = node.parentElement) {
      parts.unshift(anchorOf(node) ?? segmentOf(node))
      if (node === document.body || isUnique(parts.join(' > '), el)) break
    }
    while (parts.join(' > ').length > 120 && parts.length > 1) parts.shift()
    return parts.join(' > ')
  }

  function headingBefore(el) {
    for (let node = el; node && node !== document.body; node = node.parentElement) {
      for (let sib = node.previousElementSibling; sib; sib = sib.previousElementSibling) {
        const heading = sib.matches(HEADING) ? sib : [...sib.querySelectorAll(HEADING)].pop()
        if (heading) return flat(heading.textContent, 40)
      }
    }
    return ''
  }

  function itemOf(el) {
    const same = [...(el.parentElement?.children ?? [])].filter((c) => c.tagName === el.tagName)
    return {
      s: selectorOf(el),
      tag: tagOf(el),
      role: roleOf(el) ?? '',
      name: flat(nameOf(el), 60),
      text: flat(textOf(el), 80),
      r: box(el),
      h: headingBefore(el),
      i: Math.max(same.indexOf(el), 0),
    }
  }

  function union(items) {
    const x1 = Math.min(...items.map((i) => i.r[0]))
    const y1 = Math.min(...items.map((i) => i.r[1]))
    const x2 = Math.max(...items.map((i) => i.r[0] + i.r[2]))
    const y2 = Math.max(...items.map((i) => i.r[1] + i.r[3]))
    return [x1, y1, x2 - x1, y2 - y1]
  }

  function overlap(r, b) {
    const w = Math.min(r.right, b[0] + b[2]) - Math.max(r.left, b[0])
    const h = Math.min(r.bottom, b[1] + b[3]) - Math.max(r.top, b[1])
    return w > 0 && h > 0 ? w * h : 0
  }

  function commonAncestor(els) {
    let node = els[0].parentElement
    while (node && !els.every((el) => node.contains(el))) node = node.parentElement
    return node
  }

  function regionElements(b) {
    const picked = new Set()
    for (const el of document.body.querySelectorAll('*')) {
      if (SKIPPED.has(el.tagName) || el.closest('[data-crowbar-pick]')) continue
      const r = el.getBoundingClientRect()
      if (r.width <= 0 || r.height <= 0) continue
      if (overlap(r, b) / (r.width * r.height) < INSIDE) continue
      if (!VISUAL.has(el.tagName) && !flat(el.textContent, 1)) continue
      if (!hidden(el)) picked.add(el)
    }
    const top = [...picked].filter((el) => {
      for (let p = el.parentElement; p; p = p.parentElement) if (picked.has(p)) return false
      return true
    })
    if (top.length <= MAX_ITEMS) return top
    const parent = commonAncestor(top)
    const fills = parent && overlap(parent.getBoundingClientRect(), b) / (b[2] * b[3]) >= INSIDE
    return fills && parent !== document.body ? [parent] : top
  }

  function collect(kind, els, extra = {}) {
    const items = els.map(itemOf)
    return {
      kind,
      title: document.title,
      url: location.href,
      items: items.slice(0, MAX_ITEMS),
      more: Math.max(items.length - MAX_ITEMS, 0),
      box: extra.box ?? union(items),
      ...(extra.quote ? { quote: extra.quote } : {}),
    }
  }

  function selectedText() {
    const selection = window.getSelection?.()
    const quote = flat(selection?.toString(), 300)
    if (!quote) return null
    const node = selection.getRangeAt(0).commonAncestorContainer
    const el = node.nodeType === 1 ? node : node.parentElement
    return el ? { el, quote } : null
  }

  const swallow = (e) => {
    e.preventDefault()
    e.stopPropagation()
  }

  // The click that follows the finishing mouseup would reach the page; the next
  // mousedown means none is coming, so it ends the guard.
  function eatTrailingClick() {
    const release = () => {
      window.removeEventListener('click', eat, true)
      window.removeEventListener('mousedown', release, true)
    }
    const eat = (e) => {
      swallow(e)
      release()
    }
    window.addEventListener('click', eat, true)
    window.addEventListener('mousedown', release, true)
  }

  function pick() {
    window[PICK_KEY]?.finish(null)
    return new Promise((resolvePick) => {
      const root = document.createElement('div')
      root.setAttribute('data-crowbar-pick', '')
      root.style.cssText = 'position:fixed;inset:0;pointer-events:none;z-index:2147483647'
      const style = document.createElement('style')
      style.textContent = '* { cursor: crosshair !important; }'
      const hover = document.createElement('div')
      const drag = document.createElement('div')
      const marks = document.createElement('div')
      const paint = 'position:fixed;box-sizing:border-box;border:2px solid #3b82f6;'
      hover.style.cssText = paint
      drag.style.cssText = `${paint}background:rgba(59,130,246,.15)`
      root.append(style, hover, drag, marks)
      document.documentElement.append(root)

      const multi = new Set()
      let start = null
      let dragging = false
      let native = false
      let done = false

      const place = (node, b) => {
        node.style.display = b ? 'block' : 'none'
        if (b)
          Object.assign(node.style, {
            left: `${b[0]}px`,
            top: `${b[1]}px`,
            width: `${b[2]}px`,
            height: `${b[3]}px`,
          })
      }
      const drawMarks = () => {
        marks.replaceChildren(
          ...[...multi].map((el) => {
            const mark = document.createElement('div')
            mark.style.cssText = paint
            place(mark, box(el))
            return mark
          }),
        )
      }
      place(hover, null)
      place(drag, null)

      const finish = (result) => {
        if (done) return
        done = true
        for (const [type, fn] of listeners) window.removeEventListener(type, fn, true)
        root.remove()
        if (window[PICK_KEY] === handle) delete window[PICK_KEY]
        resolvePick(result)
      }
      const handle = { finish }
      window[PICK_KEY] = handle

      const dragBox = (e) => [
        Math.min(start.x, e.clientX),
        Math.min(start.y, e.clientY),
        Math.abs(e.clientX - start.x),
        Math.abs(e.clientY - start.y),
      ]
      const pointable = (el) =>
        el instanceof window.Element && el !== document.documentElement && el !== document.body

      const onDown = (e) => {
        if (e.button !== 0) return
        native = e.altKey || e.detail >= 2
        if (native) return e.stopPropagation()
        swallow(e)
        start = { x: e.clientX, y: e.clientY }
      }
      const onMove = (e) => {
        if (native) return
        if (start && !dragging && Math.hypot(e.clientX - start.x, e.clientY - start.y) > DRAG_PX) {
          dragging = true
        }
        if (dragging) {
          place(hover, null)
          return place(drag, dragBox(e))
        }
        place(hover, pointable(e.target) ? box(e.target) : null)
      }
      const onUp = (e) => {
        if (e.button !== 0) return
        if (native) {
          native = false
          e.stopPropagation()
          const text = selectedText()
          if (text) {
            eatTrailingClick()
            finish(collect('text', [text.el], { quote: text.quote }))
          }
          return
        }
        swallow(e)
        const wasDragging = dragging
        const region = wasDragging ? dragBox(e) : null
        dragging = false
        start = null
        place(drag, null)
        if (region) {
          const els = regionElements(region)
          if (els.length > 0) {
            eatTrailingClick()
            finish(collect('region', els, { box: region }))
          }
          return
        }
        if (!pointable(e.target)) return
        if (e.shiftKey) {
          if (!multi.delete(e.target)) multi.add(e.target)
          return drawMarks()
        }
        eatTrailingClick()
        if (multi.size === 0) return finish(collect('element', [e.target]))
        multi.add(e.target)
        finish(collect('multi', [...multi]))
      }
      const onKey = (e) => {
        if (e.key === 'Escape') {
          swallow(e)
          finish(null)
        } else if (e.key === 'Enter' && multi.size > 0) {
          swallow(e)
          finish(collect('multi', [...multi]))
        }
      }
      const listeners = [
        ['mousedown', onDown],
        ['mousemove', onMove],
        ['mouseup', onUp],
        ['click', swallow],
        ['pointerdown', (e) => !native && e.stopPropagation()],
        ['keydown', onKey],
        ['scroll', drawMarks],
        ['pagehide', () => finish(null)],
      ]
      for (const [type, fn] of listeners) window.addEventListener(type, fn, true)
    })
  }

  function cancelPick() {
    const active = window[PICK_KEY]
    active?.finish(null)
    return Boolean(active)
  }

  const agrees = (el, item) => !item.text || flat(textOf(el), 80) === item.text

  function locate(item) {
    let bySelector = null
    try {
      bySelector = item.s ? document.querySelector(item.s) : null
    } catch {
      // A selector the page no longer parses is simply not found by selector.
    }
    if (bySelector && tagOf(bySelector) === item.tag && agrees(bySelector, item)) {
      return { el: bySelector, matchedBy: 'selector' }
    }
    if (!item.text) return null
    const matches = [...document.getElementsByTagName(item.tag)].filter(
      (el) => agrees(el, item) && (!item.h || headingBefore(el) === item.h),
    )
    const el = matches.find((m) => itemOf(m).i === item.i) ?? matches[0]
    return el ? { el, matchedBy: 'fingerprint' } : null
  }

  function describeFound({ el, matchedBy }, detail) {
    const out = { found: true, ref: refOf(el), matchedBy, box: box(el) }
    if (detail === 'text') out.text = flat(textOf(el), 1000)
    if (detail === 'html') {
      out.html = el.outerHTML.slice(0, HTML_CAP)
      out.truncated = el.outerHTML.length > HTML_CAP
    }
    if (detail === 'styles') {
      const computed = getComputedStyle(el)
      out.styles = Object.fromEntries(STYLES.map((k) => [k, computed.getPropertyValue(k)]))
    }
    return out
  }

  function resolve({ items, detail = 'refs' }) {
    const list = Array.isArray(items) ? items : []
    return {
      items: list.map((item) => {
        const hit = locate(item)
        return hit ? describeFound(hit, detail) : { found: false }
      }),
    }
  }

  const ops = { pick, cancelPick, resolve }
  return (op, args) => (ops[op] ? ops[op](args ?? {}) : base(op, args))
}
