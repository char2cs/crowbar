import { createChat, createWorkspaceChat, getChat } from '../lib/fixture.mjs'
import { assertEqual, until } from '../lib/wait.mjs'

// Rename from a sidebar row's context menu keeps its inline input: the menu
// closing must not hand focus back to the row (that blur would commit and
// unmount the editor at once). Enter, Escape and blur keep their meaning, and
// double-click still opens the same editor. The rendered menu exists only on
// the browser target (Tauri pops the native one), so there only double-click runs.

const openMenuRename = (id) => {
  const row = document.querySelector(`[role="treeitem"][data-sidebar-row-id="${id}"]`)
  if (!row) return false
  row.focus()
  const r = row.getBoundingClientRect()
  const at = { bubbles: true, cancelable: true, button: 2, clientX: r.x + 20, clientY: r.y + 10 }
  row.dispatchEvent(new MouseEvent('contextmenu', at))
  return true
}
const clickRename = () => {
  const item = [...document.querySelectorAll('[role="menuitem"]')].find(
    (el) => el.textContent.trim() === 'Rename',
  )
  if (!item) return false
  item.click()
  return true
}
const doubleClickLabel = (id) => {
  const label = document.querySelector(
    `[role="treeitem"][data-sidebar-row-id="${id}"] [data-sidebar-row-label]`,
  )
  if (!label) return false
  label.dispatchEvent(new MouseEvent('dblclick', { bubbles: true, cancelable: true }))
  return true
}
/** The row's inline editor holds focus and no menu is left open. */
const editorFocused = (id) => {
  const row = document.querySelector(`[role="treeitem"][data-sidebar-row-id="${id}"]`)
  const active = document.activeElement
  return (
    !document.querySelector('[role="menu"]') &&
    active?.tagName === 'INPUT' &&
    !!row?.contains(active)
  )
}
/** Types `value` into the focused editor, then ends the edit with `end`: a key or 'blur'. */
const finishEdit = ({ value, end }) => {
  const input = document.activeElement
  const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
  setValue.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
  if (end === 'blur') input.blur()
  else input.dispatchEvent(new KeyboardEvent('keydown', { key: end, bubbles: true }))
  return true
}
const editorGone = (id) =>
  !document.querySelector(`[role="treeitem"][data-sidebar-row-id="${id}"] input`)

async function startRename(d, id, via) {
  await d.until(
    `row ${id}`,
    (rowId) => !!document.querySelector(`[data-sidebar-row-id="${rowId}"]`),
    id,
    {
      timeout: 30_000,
    },
  )
  if (via === 'menu') {
    await d.eval(openMenuRename, id)
    await d.until('the Rename menu item', clickRename)
  } else {
    await d.until('the row label to double-click', doubleClickLabel, id)
  }
  await d.until(`row ${id}'s rename input to hold focus after ${via}`, editorFocused, id)
}

async function rename(d, id, via, value, end) {
  await startRename(d, id, via)
  await d.eval(finishEdit, { value, end })
  await d.until(`row ${id}'s editor to close`, editorGone, id)
}

export default {
  name: 'menu-rename',
  async run({ d, daemon, fx, target }) {
    const fork = await createWorkspaceChat(daemon, fx)
    const base = await getChat(daemon, fx, fx.baseChatId)
    const thread = await createChat(daemon, fx, base.workspaceId)
    const { folder } = await daemon.post(`${fx.base}/chats/folders`, { name: 'Bugs', parentId: '' })
    const title = async () => (await getChat(daemon, fx, thread.id)).title
    const folderName = async () => {
      const listed = await daemon.get(`${fx.base}/chats/folders`)
      return (Array.isArray(listed) ? listed : listed.folders).find((f) => f.id === folder.id)?.name
    }
    const branch = async () => (await getChat(daemon, fx, fork.id)).worktree?.branch

    const vias = target === 'browser' ? ['menu', 'dblclick'] : ['dblclick']
    for (const via of vias) {
      await rename(d, thread.id, via, `thread via ${via}`, 'Enter')
      await until(
        `the chat title to be "thread via ${via}"`,
        async () => (await title()) === `thread via ${via}`,
      )

      await rename(d, folder.id, via, `folder via ${via}`, 'blur')
      await until(
        `the folder name to be "folder via ${via}"`,
        async () => (await folderName()) === `folder via ${via}`,
      )

      const before = await folderName()
      await rename(d, folder.id, via, 'never saved', 'Escape')
      assertEqual(await folderName(), before, 'Escape leaves the folder name as it was')
    }

    // A fork's row is id'd by its workspace; the branch rename is its own endpoint.
    const forkVia = vias[0]
    await rename(d, fork.workspaceId, forkVia, `e2e-renamed-${forkVia}`, 'Enter')
    await until(
      'the fork branch to be renamed',
      async () => (await branch()) === `e2e-renamed-${forkVia}`,
    )
  },
}
