import './oobe-gradient.css'

/** The first-run tagline, shared by the desktop OOBE and the phone's add-host screen. */
export const OOBE_TAGLINE = 'The IDE where agents do the heavy lifting.'

// The three brand colours drifting under a grain, in CSS animated by the compositor alone: a
// WebGL shader (three.js, ~280 KB gzip) is too heavy for one backdrop.
export function OobeGradient() {
  return (
    <div aria-hidden className="oobe-gradient pointer-events-none absolute inset-0 overflow-hidden">
      <div className="oobe-gradient__blob oobe-gradient__blob--teal" />
      <div className="oobe-gradient__blob oobe-gradient__blob--orange" />
      <div className="oobe-gradient__blob oobe-gradient__blob--lavender" />
      <div className="oobe-gradient__grain" />
    </div>
  )
}
