// Page-side helpers shared by the scenarios. Every function passed to
// driver.eval runs in the webview, so it must be self-contained.

/**
 * Opens (or focuses) a chat by clicking its sidebar tree row until its Recents
 * row exists. The click is repeated because a row for a chat the sidebar has
 * not finished loading ignores it; each repeat is a no-op once the view is up.
 */
export async function openChat(driver, chatId) {
  await driver.until(
    `Recents row of chat ${chatId}`,
    (id) => {
      if (document.querySelector(`[data-testid="recents-row-${id}"]`)) return true
      document.querySelector(`[data-sidebar-row-id="${id}"]`)?.click()
      return false
    },
    chatId,
    { timeout: 30_000 },
  )
}

/** Brings a chat's view forward by clicking its Recents row. */
export function focusRecent(driver, chatId) {
  return driver.eval((id) => {
    const row = document.querySelector(`[data-testid="recents-row-${id}"] [role="treeitem"]`)
    if (!row) return false
    row.click()
    return true
  }, chatId)
}

/** Closes every view through Recents' own x, leaving a clean slate for the next scenario. */
export async function closeAllViews(driver) {
  const count = () => document.querySelectorAll('[data-testid^="recents-row-"]').length
  for (;;) {
    const before = await driver.eval(count)
    if (!before) return
    await driver.eval(() => {
      document.querySelector('[data-testid^="recents-row-"] button[aria-label^="Close "]').click()
      return true
    })
    await driver.until(
      'a Recents row to disappear',
      (n) => document.querySelectorAll('[data-testid^="recents-row-"]').length < n,
      before,
    )
  }
}

/** Reloads the page and waits for the shell to mount again. */
export async function reload(driver, { home = false } = {}) {
  await driver.eval((toHome) => {
    if (toHome) location.hash = '#/'
    window.__e2eReloading = true
    setTimeout(() => location.reload(), 0)
    return true
  }, home)
  await driver.until(
    'the page to reload',
    () => !window.__e2eReloading && !!document.querySelector('[data-slot="console-dock"]'),
    null,
    { timeout: 60_000 },
  )
}
