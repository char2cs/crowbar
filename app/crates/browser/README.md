# browser

## Owns

What the browser pane's address bar means, which URLs a browser pane may load, the JavaScript the
shell injects into a page for agent operations and element picking, and the page state the native
chrome shows (address, title, loading, back/forward).

## Public interface

- `resolve_address`: turns typed text into a URL or a search.
- `BROWSER_PANE_TYPE`: the pane type id of the browser pane.
- `PageUrl`: the only URL form a browser pane loads.
- `SearchEngine`: search URL templates.
- `agent_script`, `eval_script`, `pick_script`, `cancel_pick_script`: scripts injected into the page.
- `PickReport`, `PointerItem`, `pointer_text`, `pick_count`: elements the user picked, as chat text.
- `PageUpdate`, `AgentAnswer`, `PageState`, `tab_name`: engine reports folded into what the chrome shows.
- `Error`: a URL a browser pane refuses.

## Depends on

No workspace crate (percent-encoding, serde, serde_json, thiserror, url).
