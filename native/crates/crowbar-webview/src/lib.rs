#![forbid(unsafe_code)]

//! `crowbar-webview` — webview panes and windows.
//!
//! Scaffold only (item 0.1).
//!
//! Dependency contract (§4.2): `crowbar-ui`, `crowbar-state`.
//!
//! Hosts the surfaces §5.3 keeps in a webview permanently — Plate, mermaid,
//! katex, HTML preview — pointed at the same `web/` build the daemon already
//! serves, over the §9.4 loopback listener (item 0.7).
