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

const rowIdByLabel = (label) =>
  document.querySelector(`[role="treeitem"][aria-label="${label}"]`)?.dataset.sidebarRowId ?? null

export default {
  name: 'menu-rename',
  async run({ d, daemon, fx, target }) {
    const fork = await createWorkspaceChat(daemon, fx)
    const thread = await createChat(daemon, fx, fork.workspaceId)
    const { folder } = await daemon.post(`${fx.base}/chats/folders`, { name: 'Bugs', parentId: '' })
    const repoRowId = await d.until('the repo header row', rowIdByLabel, 'e2e-repo', {
      timeout: 30_000,
    })
    const chatTitle = (id) => async () => (await getChat(daemon, fx, id)).title
    // A folder is a chat row on the daemon: its name is its title.
    const folderName = async () =>
      (await daemon.get(`${fx.base}/chats/folders`)).find((f) => f.id === folder.id)?.title
    const repoName = async () =>
      (await daemon.get(`/v0/projects/${fx.projectId}/repos`)).find((r) => r.id === fx.repoId)?.name
    // Each kind of row the menu renames, each ending the edit a different way.
    const rows = [
      { kind: 'thread', id: thread.id, read: chatTitle(thread.id), end: 'Enter' },
      { kind: 'workspace', id: fork.id, read: chatTitle(fork.id), end: 'Enter' },
      { kind: 'folder', id: folder.id, read: folderName, end: 'blur' },
      { kind: 'repo', id: repoRowId, read: repoName, end: 'Enter' },
    ]

    for (const via of target === 'browser' ? ['menu', 'dblclick'] : ['dblclick']) {
      for (const row of rows) {
        const name = `${row.kind}-via-${via}`
        await rename(d, row.id, via, name, row.end)
        await until(
          `the ${row.kind} to be named "${name}"`,
          async () => (await row.read()) === name,
        )

        await rename(d, row.id, via, 'never-saved', 'Escape')
        assertEqual(await row.read(), name, `Escape leaves the ${row.kind} named ${name}`)
      }
    }
  },
}
