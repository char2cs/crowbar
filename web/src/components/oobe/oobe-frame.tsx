import './oobe-gradient.css'

/** The first-run tagline, shared by the desktop OOBE and the phone's add-host screen. */
export const OOBE_TAGLINE = 'The IDE where agents do the heavy lifting.'

// A CSS stand-in for the WebGL shader gradient this screen used to render
// (three.js + react-three-fiber, ~280 KB gzip for one backdrop): the same three
// colors drifting under a grain, animated by the compositor alone.
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
