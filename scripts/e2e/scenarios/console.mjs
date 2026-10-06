import { createWorkspaceChat, hook } from '../lib/fixture.mjs'
import { reload } from '../lib/ui.mjs'
import { assert, assertEqual } from '../lib/wait.mjs'

// The daemon log console: open/close paths, overlay vs push, the hotkey and
// menu event, live lines, the restart note and dock preference persistence.

const state = () => {
  const panel = document.getElementById('console-panel')
  const app = document.querySelector('[data-slot="console-dock"]').firstElementChild
  return {
    open: panel.dataset.open === 'true',
    dock: panel.dataset.dock,
    mode: panel.dataset.mode,
    appHeight: Math.round(app.getBoundingClientRect().height),
    windowHeight: window.innerHeight,
    size: Number(panel.querySelector('[data-slot="console-resize"]').getAttribute('aria-valuenow')),
  }
}

const clickBanner = () => {
  const banner = [...document.querySelectorAll('button[aria-controls="console-panel"]')].find(
    (b) => b.offsetParent,
  )
  banner.click()
  return true
}
const clickHeaderButton = (prefix) => {
  const button = [...document.querySelectorAll('#console-panel header button')].find((b) =>
    b.getAttribute('aria-label')?.startsWith(prefix),
  )
  button.click()
  return true
}
const pressEscapeInPanel = () => {
  const log = document.querySelector('#console-panel [data-slot="console-log"]')
  log.dispatchEvent(
    new KeyboardEvent('keydown', {
      key: 'Escape',
      bubbles: true,
      cancelable: true,
    }),
  )
  return true
}
const pressOutside = () => {
  const outside = document.querySelector('[data-slot="console-dock"]').firstElementChild
  outside.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, pointerId: 1, button: 0 }))
  return true
}
// Mod+` as a real keydown. The chord binds Cmd on macOS and Ctrl elsewhere.
const pressHotkey = () => {
  const mac = /Mac/i.test(navigator.platform)
  window.dispatchEvent(
    new KeyboardEvent('keydown', {
      key: '`',
      code: 'Backquote',
      metaKey: mac,
      ctrlKey: !mac,
      bubbles: true,
      cancelable: true,
    }),
  )
  return true
}
const nudgeSize = (key) => {
  const handle = document.querySelector('#console-panel [data-slot="console-resize"]')
  handle.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }))
  return true
}
const clearPrefs = () => {
  for (const key of ['console-dock', 'console-mode', 'console-size']) localStorage.removeItem(key)
  return true
}
const hasLog = (needle) =>
  [...document.querySelectorAll('#console-panel [data-slot="log-row"]')].some((row) =>
    row.textContent.includes(needle),
  )
const hasNote = (needle) =>
  [...document.querySelectorAll('#console-panel [data-slot="console-note"]')].some((n) =>
    n.textContent.includes(needle),
  )

export default {
  name: 'console',
  async run({ d, inst, daemon, fx, target }) {
    const open = (want) =>
      d.until(
        `console open=${want}`,
        (w) => document.getElementById('console-panel').dataset.open === String(w),
        want,
      )

    await d.eval(clearPrefs)
    await reload(d)
    assertEqual((await d.eval(state)).open, false, 'the console starts closed')

    // Banner click toggles.
    await d.eval(clickBanner)
    await open(true)
    await d.eval(clickBanner)
    await open(false)

    // Escape closes.
    await d.eval(clickBanner)
    await open(true)
    await d.eval(pressEscapeInPanel)
    await open(false)

    // Overlay: a press outside closes it and it floats (the app keeps its height).
    await d.eval(clickBanner)
    await open(true)
    let s = await d.eval(state)
    assertEqual(s.mode, 'overlay', 'default mode is overlay')
    assertEqual(s.appHeight, s.windowHeight, 'an overlay does not shrink the app')
    await d.eval(pressOutside)
    await open(false)

    // Push: a press outside leaves it open and the app shrinks beside it.
    await d.eval(clickBanner)
    await open(true)
    await d.eval(clickHeaderButton, 'Mode:')
    await d.until(
      'push mode',
      () => document.getElementById('console-panel').dataset.mode === 'push',
    )
    await d.eval(pressOutside)
    await d.until('the app to share the window', () => {
      const app = document.querySelector('[data-slot="console-dock"]').firstElementChild
      return app.getBoundingClientRect().height < window.innerHeight - 50
    })
    assert((await d.eval(state)).open, 'a press outside must not close a pushed console')
    await d.eval(pressEscapeInPanel)
    await open(false)

    // Hotkey and (desktop only) the native menu event.
    await d.eval(pressHotkey)
    await open(true)
    await d.eval(pressHotkey)
    await open(false)
    if (target === 'tauri') {
      await d.emit('console:toggle')
      await open(true)
      await d.emit('console:toggle')
      await open(false)
    }

    // Live lines: a turn starting is logged once the console is open.
    const chat = await createWorkspaceChat(daemon, fx)
    await d.eval(clickBanner)
    await open(true)
    await hook(daemon, fx, chat, 'session_start', { session_id: 's-console' })
    await hook(daemon, fx, chat, 'user_prompt', {
      session_id: 's-console',
      prompt: 'log me',
    })
    await d.until('a "turn started" line for the chat', hasLog, 'turn started')

    // A daemon restart is announced and the stream resumes.
    await inst.restartDaemon()
    await d.until('the "Daemon restarted" note', hasNote, 'Daemon restarted')
    await d.eval(pressEscapeInPanel)
    await open(false)

    // Dock, mode and size survive a full relaunch of the app.
    await d.eval(clickBanner)
    await open(true)
    await d.eval(clickHeaderButton, 'Dock position:')
    await d.until(
      'dock moved right',
      () => document.getElementById('console-panel').dataset.dock === 'right',
    )
    const before = (await d.eval(state)).size
    for (let i = 0; i < 3; i++) await d.eval(nudgeSize, 'ArrowLeft')
    await d.until(
      'size to grow',
      (n) =>
        Number(
          document
            .querySelector('#console-panel [data-slot="console-resize"]')
            .getAttribute('aria-valuenow'),
        ) > n,
      before,
    )
    const want = await d.eval(state)
    assertEqual([want.dock, want.mode], ['right', 'push'], 'dock and mode before relaunch')

    assertEqual(
      await d.eval(() => localStorage.getItem('console-dock')),
      'right',
      'the dock preference was written to storage',
    )
    await inst.relaunchApp()
    const got = await d.eval(state)
    assertEqual(
      [got.dock, got.mode, got.size],
      [want.dock, want.mode, want.size],
      'dock, mode and size after a full relaunch',
    )
    await d.eval(clearPrefs)
  },
}
