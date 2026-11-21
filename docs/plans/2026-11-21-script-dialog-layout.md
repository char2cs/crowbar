# Script dialogs in the foreign page window (ED-087, DLG-017)

Owner: webview crate (ROUTE lane). Rows: ED-087 and DLG-017 (the untitled save hang).

## Cause

`wry::build_as_child` wraps the shell's X window in a `GtkWindow`. A foreign GDK window gets no
frame clock, so GTK's idle sizer never runs and a queued resize is never allocated. WebKit's
default `script-dialog` handler parents its overlay (`WebKitScriptDialogImpl`) on the web view
and queues a resize. Nothing allocates the overlay, so it is never drawn and the page waits in
`prompt()`. The next `set_bounds` calls `size_allocate`, and only then does the prompt appear.

Develop's Tauri window shows the same in-view overlay. Once it is laid out, WebKit's own
overlay is the parity answer for prompt, confirm and alert, so we add no dialog code.

## Rejected: embedding each page in a GtkPlug

A plug has a frame clock, and it does make the prompt show. It was dropped because, under
openbox (CI's GUI job) and with `WEBKIT_DISABLE_DMABUF_RENDERER` or
`WEBKIT_DISABLE_COMPOSITING_MODE` (the rig, and many NVIDIA setups), it broke other things:

- the plug's X window took keys while the shell held focus;
- pages painted once and then never again until their size changed;
- the plug's reported geometry did not follow its bounds.

## Change: the web view lays out what takes focus inside it

With no frame clock, our placement is the only thing that ever allocates the page, so the web
view owns this allocation. WebKit's overlay grabs focus when it maps: the default button for an
alert, the entry for a prompt. That grab emits `set-focus` on the page's `GtkWindow`
synchronously, inside WebKit's own `AddDialog`. Our handler re-allocates the web view at its
current allocation. `webkitWebViewBaseSizeAllocate` then centres and sizes the dialog, and it
draws.

The trigger is a GTK event, not an idle, a tick or a retry. Nothing else is stored: the
allocation already lives in GTK.

## Tests

The tests live in `webview/tests/host.rs` and run on a parent with a black background, so stale
pixels cannot pass a check:

- `a_prompt_shows_over_its_page_and_answers_it`: the page goes dark, `prompt()` runs, lit pixels
  must appear, and Return must answer "Untitled". Red before the change.
- `a_spare_page_paints_over_the_page_it_replaces`: a regression guard for the placement path.

Every webview display test runs three ways: bare Xvfb, Xvfb with openbox, and Xvfb with openbox
under the rig's WebKit environment.
