/** Fills the page with the pane surface colour: on Linux the body wash is the dark sidebar chrome, and a pane page has no container around it that paints the content area. */
export function paintPaneSurface(body: HTMLElement): void {
  body.style.backgroundColor = 'var(--pane-chrome-bg)'
}
