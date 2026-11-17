# One owner for a browser pane's address

## Problem

The address of a browser pane lives twice. `BrowserPages` (`app/desktop/app/src/browser_pages.rs`) folds the
engine's `PageUpdate`s into a live `PageState` (url, title, loading); the layout's `PaneState` keeps
`{"url": ...}`, written by `save_address` on every load end so the pane reopens where it was.
`address_of` (`browser_tabs.rs`) reads the live url and falls back to the saved one. The two agree
only after a load end: during a navigation the live url is ahead, after a restart only the saved one exists.

## Why it was not removed in the audit fix

- The layout copy is the persisted one (the pane reopens from it); the live copy is the only one that knows
  `loading` and the title. Dropping either breaks a reader: the agent's `browser_tabs` list of a window that
  has not woken its views yet reads the saved url, and `agent_in_page` decides "this tab has no page" from it
  before any view exists.
- Making the layout the single owner would write layout state (and its save and undo history) on every
  `Loading` update and every redirect; making `BrowserPages` the single owner needs it to be seeded from the
  layout at open and at restore, which is a second copy by another name.

## Plan

1. Decide the owner: the layout's `PaneState` stays the owner of the address (what reopens), `BrowserPages`
   owns only what the layout never stores (`loading`, title, history flags).
2. `PageState` loses `url`; a live url is read from the engine's last committed address that `save_address`
   already writes on load end. For the window between a navigation start and its load end the pane shows the
   address the user or agent asked for, which `BrowseAction::Go` can write to the layout once, at the request,
   not on every update.
3. `address_of` then reads the layout alone; `TabInfo.url` is that address, `TabInfo.loading` and `title` come
   from `BrowserPages`.
4. Test first: a navigation request writes the asked address; a redirect's load end overwrites it; a restored
   layout lists its tabs with their saved urls and `loading: false` without any `PageUpdate`.

## Risks

- Layout writes per navigation request must not enter the undo history (`SetPaneState` already does not).
- A page that redirects to a blank or refused address must leave the saved address untouched.
