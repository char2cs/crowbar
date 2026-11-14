import { reload } from '../lib/ui.mjs'
import { assert } from '../lib/wait.mjs'

// The sidebar build band for the nightly channel, in light and dark. Channel
// and theme are real persisted settings (one localStorage key per setting),
// applied by reloading, exactly as a restart would.

const setSettings = (settings) => {
  for (const [key, value] of Object.entries(settings))
    localStorage.setItem(`crowbar:settings:${key}`, JSON.stringify(value))
  return true
}
const clearSettings = () => {
  for (const key of ['buildBadgeOverride', 'themeMode'])
    localStorage.removeItem(`crowbar:settings:${key}`)
  return true
}
const band = () => {
  const el = document.querySelector('[data-slot="build-badge-band"]')
  if (!el) return null
  const fill = el.firstElementChild
  const rect = el.getBoundingClientRect()
  const badge = [...document.querySelectorAll('button[aria-controls="console-panel"]')].find(
    (b) => b.offsetParent,
  )
  return {
    channel: el.dataset.channel,
    dark: document.documentElement.classList.contains('dark'),
    color: fill ? getComputedStyle(fill).backgroundColor : '',
    photo: document.querySelector('[data-band-photo]')?.dataset.bandPhoto ?? null,
    photoLoaded: !!document.querySelector('[data-band-photo]')?.naturalWidth,
    visible: rect.width > 0 && rect.height > 0,
    label: badge?.textContent ?? '',
  }
}

export default {
  name: 'sidebar-band',
  async run({ d }) {
    const seen = {}
    for (const mode of ['light', 'dark']) {
      await d.eval(setSettings, {
        buildBadgeOverride: 'nightly',
        themeMode: mode,
      })
      await reload(d)
      await d.until(
        `the nightly band in ${mode}`,
        () => !!document.querySelector('[data-slot="build-badge-band"][data-channel="nightly"]'),
      )
      await d.until(
        `the ${mode} band photo to decode`,
        () => !!document.querySelector('[data-band-photo]')?.naturalWidth,
      )
      const got = await d.eval(band)
      assert(got.visible, `the band is laid out in ${mode}`)
      assert(
        got.dark === (mode === 'dark'),
        `the ${mode} theme is applied (dark class ${got.dark})`,
      )
      assert(got.photo === mode, `the ${mode} band shows its ${mode} photo (got ${got.photo})`)
      assert(got.photoLoaded, `the ${mode} photo decoded`)
      assert(got.label.length > 0, `the badge has a label in ${mode}`)
      seen[mode] = got
    }
    assert(seen.light.color !== seen.dark.color, 'light and dark bands paint different grounds')
    assert(
      seen.light.label !== seen.dark.label,
      'the nightly label differs between light ("daily") and dark',
    )
    await d.eval(clearSettings)
    await reload(d)
  },
}
