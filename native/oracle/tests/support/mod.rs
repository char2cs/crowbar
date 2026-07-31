//! Fixtures shared by the integration tests.
//!
//! The reference snapshot is the Phase 1 gate target reduced to three anchors,
//! chosen so that between them they carry **every** field ANCHORS.md §3
//! defines: a pseudo-backed row background with a radius and a border, a
//! text-free icon, and a truncating filename span with the full text group.
//!
//! Every case in the differ tests is produced by replacing exactly one
//! substring of it, so a case differs from the reference in exactly one field
//! and nothing else. [`native`] asserts the substring is unique, so a fixture
//! edit that makes a pattern ambiguous fails loudly instead of silently
//! mutating two fields.

#![allow(dead_code)] // Not every fixture is used by every test binary.

/// The reference (React-side) snapshot.
pub const REFERENCE: &str = r##"{
  "schema": 1,
  "surface": "git-status-row",
  "state": { "width": 320, "theme": "dark", "content": "overflow",
             "flags": ["selected", "hover"] },
  "root": "git-row-item",
  "anchors": [
    { "id": "git-row-item",
      "bounds": { "x": 0.0, "y": 0.0, "w": 320.0, "h": 24.0 },
      "bg": "#1e2228ff",
      "visible": true,
      "radius": 2.0,
      "border": { "w": 1.0, "color": "#00000000" } },
    { "id": "git-row-icon",
      "bounds": { "x": 8.0, "y": 5.0, "w": 14.0, "h": 14.0 },
      "bg": "#00000000",
      "visible": true },
    { "id": "git-row-name",
      "bounds": { "x": 30.0, "y": 4.0, "w": 118.0, "h": 16.0 },
      "fg": "#c8ccd4ff",
      "bg": "#00000000",
      "text": "resolve-terminal-connection.ts",
      "text_width": 186.5,
      "clipped": true,
      "font": { "size": 13.0, "weight": 500, "family": "Inter",
                "line_height": 17.5 },
      "visible": true }
  ]
}"##;

/// A snapshot with no anchors at all.
pub const EMPTY: &str = r#"{
  "schema": 1,
  "surface": "git-status-row",
  "state": { "width": 320, "theme": "dark", "content": "overflow",
             "flags": ["selected", "hover"] },
  "root": "git-row-item",
  "anchors": []
}"#;

/// The reference with exactly one substring replaced.
///
/// Panics if `from` does not occur exactly once, which turns an ambiguous
/// fixture edit into a test failure rather than a silently doubled mutation.
#[must_use]
pub fn native(from: &str, to: &str) -> String {
    let hits = REFERENCE.matches(from).count();
    assert_eq!(
        hits, 1,
        "fixture pattern {from:?} occurs {hits} times, not once"
    );
    REFERENCE.replacen(from, to, 1)
}
