//! Opening a large file in the editor and typing at its end, through gpui's test platform with
//! the Linux text system shaping the lines (decision D8 of the native-editor plan).
//!
//! Run with `cargo bench -p editorview --bench open_large`.

mod adapters {
    pub(crate) mod stopwatch;
}

use std::io::Write;
use std::sync::Arc;

use adapters::stopwatch::Stopwatch;
use editorview::{Access, Document, DocumentOpening, EditorView, Opening};
use gpui::{AppContext as _, Entity, TestAppContext, TestDispatcher, VisualTestContext};

const LINE_COUNTS: [u32; 3] = [10_000, 50_000, 200_000];
const KEYSTROKES: u32 = 50;
const FALLBACK_FONT: &str = "IBM Plex Sans";
const MILLIS_PER_SECOND: f64 = 1e3;

fn line_of(index: u32) -> String {
    format!(
        "export const value{index} = compute({index}, \"item-{index}\");
"
    )
}

fn source(lines: u32) -> String {
    (0..lines).map(line_of).collect()
}

fn opened(
    lines: u32,
    cx: &mut TestAppContext,
) -> (Entity<EditorView>, &mut VisualTestContext) {
    let document = DocumentOpening {
        text: source(lines),
        path: "src/large.ts".to_owned(),
        saved: None,
    };
    let opening = Opening {
        line: Some(lines),
        access: Access::Editable,
    };
    let opened = cx.add_window_view(|window, cx| {
        let document = cx.new(|cx| Document::new(document, None, cx));
        EditorView::new(document, opening, window, cx)
    });
    opened.1.run_until_parked();
    opened
}

fn bench_lines(
    lines: u32,
    out: &mut impl Write,
) -> std::io::Result<()> {
    let text_system = Arc::new(gpui_wgpu::CosmicTextSystem::new(FALLBACK_FONT));
    let mut cx = TestAppContext::build_with_text_system(TestDispatcher::new(0), None, text_system);
    cx.update(ui::init).map_err(std::io::Error::other)?;
    cx.update(editorview::init);
    let opening = Stopwatch::start();
    let (_view, window) = opened(lines, &mut cx);
    let open_ms = opening.elapsed_seconds() * MILLIS_PER_SECOND;
    let (mut total, mut worst) = (0.0_f64, 0.0_f64);
    for _ in 0..KEYSTROKES {
        let typing = Stopwatch::start();
        window.simulate_input("x");
        let took = typing.elapsed_seconds() * MILLIS_PER_SECOND;
        total += took;
        worst = worst.max(took);
    }
    let mean = total / f64::from(KEYSTROKES);
    writeln!(
        out,
        "{lines:>7} lines: open {open_ms:.0} ms, keystroke mean {mean:.1} ms, worst {worst:.1} ms"
    )
}

fn main() -> std::io::Result<()> {
    let mut console = std::io::stdout().lock();
    for lines in LINE_COUNTS {
        bench_lines(lines, &mut console)?;
    }
    Ok(())
}
