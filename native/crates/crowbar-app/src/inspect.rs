//! Reading the **running** app back without looking at it.
//!
//! The Rust half of what the Tauri MCP bridge did for the React app —
//! `webview_dom_snapshot` and friends — minus the screenshot. Screen capture
//! needs an OS permission no process in this tree can grant itself, and a
//! workflow where a human squints at a window does not scale and is not
//! evidence anyone can diff.
//!
//! # What this is, in one line
//!
//! `cargo run -p crowbar-app --features driver -- --inspect` opens the **real**
//! shell against the **real** daemon, waits for the frame to settle, prints the
//! laid-out element tree as JSON, and quits.
//!
//! # Why it measures the shipping view rather than a copy
//!
//! Because `crowbar_ui::anchor` was built for exactly this: every surface takes
//! an [`AnchorSink`] and the binary decides which one it gets. The shipping
//! path passes `Unanchored`, whose methods are the identity, so the element
//! tree is **byte-for-byte the same tree** either way. Building a second,
//! inspectable view instead would mean the thing measured is not the thing
//! shipped — which is the failure this whole arrangement exists to prevent.
//!
//! # Why the frame has to settle first
//!
//! A window's first draw is one draw. Anything deferred — a popup that sizes
//! itself in `prepaint`, a stream frame that has not arrived — is not in it,
//! and a snapshot taken there reports a window that never existed for a user.
//! [`crowbar_driver::on_settled_frame`] fires on the first completed draw that
//! reproduced the previous one's anchors, which is the same signal the capture
//! harness stops on.

use std::rc::Rc;

use crowbar_driver::{Observation, Paint, RawAnchor, hex};
use crowbar_ui::AnchorSink;
use gpui::App;

use crate::driver_anchors::{AppAnchors, fold_text_halves};

/// How long an inspection waits for the daemon before reporting anyway.
///
/// Without a deadline the run waits for ever when the daemon is down or the
/// home is empty, which reads as the tool hanging rather than as a daemon that
/// is not running.
pub const DEADLINE: std::time::Duration = std::time::Duration::from_secs(8);

/// The argument that selects this mode.
pub const FLAG: &str = "--inspect";

/// Whether `args` asks for an inspection run.
pub fn requested(args: &[String]) -> bool {
    args.iter().any(|arg| arg == FLAG)
}

/// One element, as the inspector reports it.
///
/// Deliberately flat and JSON-shaped: the consumer is a tool reading stdout,
/// and a nested tree would make "is this row visible and where" a traversal
/// rather than a grep.
#[derive(Debug, serde::Serialize)]
pub struct Element {
    /// The anchor's stable semantic id.
    pub id: String,
    /// Window-relative left edge, logical pixels.
    pub x: f32,
    /// Window-relative top edge.
    pub y: f32,
    /// Border-box width.
    pub w: f32,
    /// Border-box height.
    pub h: f32,
    /// Whether gpui would actually paint it.
    pub visible: bool,
    /// The string it paints, when it paints one.
    pub text: Option<String>,
    /// Its own background fill, in the contract's `#rrggbbaa` spelling.
    /// `null` where gpui would paint no background at all — which is
    /// different from painting transparent, and the difference is exactly the
    /// kind of thing a styling bug hides in.
    pub bg: Option<String>,
    /// Corner radius, logical pixels.
    pub radius: f32,
    /// Border width, logical pixels.
    pub border_w: f32,
    /// Border colour, or `null` where gpui would paint no border.
    pub border_color: Option<String>,
    /// The colour the text is painted in.
    pub text_color: Option<String>,
    /// Font size, weight, family and line height — the four things a "the
    /// styling is wrong" report needs and geometry alone cannot answer.
    pub font_size: Option<f32>,
    /// Font weight.
    pub font_weight: Option<f32>,
    /// Font family, as resolved.
    pub font_family: Option<String>,
    /// Line height, logical pixels.
    pub line_height: Option<f32>,
}

impl Element {
    fn of(record: &RawAnchor) -> Self {
        Self {
            id: record.id.to_string(),
            x: f32::from(record.bounds.origin.x),
            y: f32::from(record.bounds.origin.y),
            w: f32::from(record.bounds.size.width),
            h: f32::from(record.bounds.size.height),
            visible: record.visible,
            text: record.text.as_ref().map(|text| text.content.to_string()),
            bg: match record.background {
                Paint::Solid(color) => Some(hex(color)),
                Paint::None => None,
                Paint::Unrepresentable => Some("unrepresentable".to_owned()),
            },
            radius: f32::from(record.radius),
            border_w: f32::from(record.border_width),
            border_color: record.border_color.map(hex),
            text_color: record.text.as_ref().map(|text| hex(text.color)),
            font_size: record.text.as_ref().map(|text| f32::from(text.font.size)),
            font_weight: record.text.as_ref().map(|text| text.font.weight),
            font_family: record
                .text
                .as_ref()
                .map(|text| text.font.family.to_string()),
            line_height: record
                .text
                .as_ref()
                .map(|text| f32::from(text.font.line_height)),
        }
    }
}

/// What one inspection run reports.
#[derive(Debug, serde::Serialize)]
pub struct Report {
    /// Whether the frame settled, or the run gave up watching it move.
    pub settled: bool,
    /// The window's own logical size, so a caller can tell "off-screen" from
    /// "zero-sized".
    pub window: [f32; 2],
    /// Every anchor the frame recorded.
    pub elements: Vec<Element>,
}

/// The sink an inspection run renders through.
#[must_use]
pub fn sink() -> Rc<dyn AnchorSink> {
    Rc::new(AppAnchors)
}

/// Print `report` as JSON on stdout, and quit.
///
/// stdout, not a file: the caller is a tool that ran this process, and a path
/// it then has to find is one more thing to get wrong.
pub fn emit(report: &Report, cx: &mut App) {
    match serde_json::to_string_pretty(report) {
        Ok(json) => println!("{json}"),
        Err(err) => eprintln!("crowbar-app: could not serialise the inspection: {err}"),
    }
    cx.quit();
}

/// Build the report from a settled frame's records.
#[must_use]
pub fn report(observation: Observation, window: [f32; 2], records: &[RawAnchor]) -> Report {
    Report {
        settled: matches!(observation, Observation::Settled),
        window,
        elements: fold_text_halves(records.to_vec())
            .iter()
            .map(Element::of)
            .collect(),
    }
}
