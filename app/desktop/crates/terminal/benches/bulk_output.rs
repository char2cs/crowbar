#![allow(
    clippy::print_stdout,
    reason = "a benchmark reports its measurements on the terminal"
)]
#![allow(
    clippy::cast_precision_loss,
    reason = "byte and iteration counts convert to floats for a rate"
)]
//! Throughput of the hot paths: feeding bulk output through the emulator and building render snapshots.
//!
//! Run with `cargo bench -p terminal --bench bulk_output`.

use std::hint::black_box;
use std::time::Instant;

use terminal::{CursorShape, DaemonFrame, Rgb, Terminal, TerminalConfig, TerminalPalette};

const COLUMNS: usize = 120;
const LINES: usize = 40;
const SCROLLBACK: usize = 10_000;
const CHUNK_BYTES: usize = 16_384;
const TOTAL_BYTES: usize = 32 * 1_024 * 1_024;
const SNAPSHOTS: usize = 500;
const ANSI_COLORS: usize = 16;
const COLOR_CYCLE: usize = 7;
const SNAPSHOT_FILL_CHUNKS: usize = 4;
const MICROS_PER_SECOND: f64 = 1e6;

fn terminal() -> Terminal {
    let gray = Rgb::from_hex(0x80_8080);
    let palette = TerminalPalette {
        foreground: gray,
        background: Rgb::from_hex(0),
        cursor: gray,
        ansi: [gray; ANSI_COLORS],
    };
    Terminal::new(TerminalConfig {
        columns: COLUMNS,
        lines: LINES,
        scrollback: SCROLLBACK,
        cursor_shape: CursorShape::Block,
        cursor_blink: false,
        palette,
    })
}

fn colored_line(index: usize) -> String {
    let color = index % COLOR_CYCLE + 1;
    format!("\x1b[3{color}mline {index:08} the quick brown fox jumps over the lazy dog \x1b[0m\r\n")
}

fn colored_lines(bytes: usize) -> Vec<u8> {
    let line_bytes = colored_line(0).len();
    (0..bytes.div_ceil(line_bytes))
        .flat_map(|index| colored_line(index).into_bytes())
        .collect()
}

fn bench_feed() {
    let data = colored_lines(TOTAL_BYTES);
    let mut terminal = terminal();
    let started = Instant::now();
    for chunk in data.chunks(CHUNK_BYTES) {
        terminal.apply(&DaemonFrame::Output(chunk.to_vec()));
    }
    let seconds = started.elapsed().as_secs_f64();
    let megabytes = data.len() as f64 / (1_024.0 * 1_024.0);
    println!(
        "apply   {megabytes:.1} MiB in {seconds:.3} s = {:.1} MiB/s",
        megabytes / seconds
    );
}

fn bench_snapshot() {
    let mut terminal = terminal();
    terminal.apply(&DaemonFrame::Output(colored_lines(
        CHUNK_BYTES * SNAPSHOT_FILL_CHUNKS,
    )));
    let started = Instant::now();
    for _ in 0..SNAPSHOTS {
        black_box(terminal.snapshot());
    }
    let micros = started.elapsed().as_secs_f64() * MICROS_PER_SECOND / SNAPSHOTS as f64;
    println!("snapshot {COLUMNS}x{LINES}: {micros:.1} us each");
}

fn main() {
    bench_feed();
    bench_snapshot();
}
