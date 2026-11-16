# Linux: a web view's first frame can be black

A pane web view shows a short black frame before its page paints on Linux.

## What was checked

`wry::WebViewExtUnix::webview()` returns the `webkit2gtk::WebView`, and `WebViewExt::set_background_color` is reachable through it. It is already applied: wry's own `WebView::set_background_color` is that call, and the shell uses it at creation (`WebViewConfig::background`) and when the theme changes (`GtkState::set_background`). The black frame remains, so it is not WebKit's page background.

## Cause

The black is the X window under the page: the child window wry creates inside the GPUI window is painted with the server's default black until WebKit's first expose. wry has no hook for that window's background, and the only way to reach it from the webview handle is the gdk window API, which is deprecated.

## Decision

Known limitation on Linux until wry exposes the child window background or the shell owns that window. Revisit when moving to a wry release that does, or if the shell starts creating the child window itself.
