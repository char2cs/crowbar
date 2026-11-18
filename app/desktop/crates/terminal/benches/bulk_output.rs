//! Throughput of the hot paths: feeding bulk output through the emulator and building render snapshots.
//!
//! Run with `cargo bench -p terminal --bench bulk_output`.

mod adapters;

use std::hint::black_box;
use std::io::Write;

use adapters::stopwatch::Stopwatch;
use terminal::{Config, CursorShape, DaemonFrame, Palette, Rgb, Terminal};

const COLUMNS: usize = 120;
const LINES: usize = 40;
const SCROLLBACK: usize = 10_000;
const CHUNK_BYTES: usize = 16_384;
const BYTES_PER_MEBIBYTE: f64 = 1_048_576.0;
const TOTAL_BYTES: usize = 32 * 1_048_576;
const SNAPSHOTS: u32 = 500;
const GRAY: u32 = 0x80_8080;
const ANSI_COLORS: usize = 16;
const COLOR_CYCLE: usize = 7;
const SNAPSHOT_FILL_CHUNKS: usize = 4;
const MICROS_PER_SECOND: f64 = 1e6;

fn terminal() -> Terminal {
    let gray = Rgb::from_hex(GRAY);
    let palette = Palette {
        foreground: gray,
        background: Rgb::from_hex(0),
        cursor: gray,
        ansi: [gray; ANSI_COLORS],
    };
    Terminal::new(Config {
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

fn bench_feed(out: &mut impl Write) -> std::io::Result<()> {
    let data = colored_lines(TOTAL_BYTES);
    let mut terminal = terminal();
    let stopwatch = Stopwatch::start();
    for chunk in data.chunks(CHUNK_BYTES) {
        terminal.apply(&DaemonFrame::Output(chunk.to_vec()));
    }
    let seconds = stopwatch.elapsed_seconds();
    let megabytes = f64::from(u32::try_from(data.len()).unwrap_or(u32::MAX)) / BYTES_PER_MEBIBYTE;
    writeln!(
        out,
        "apply   {megabytes:.1} MiB in {seconds:.3} s = {:.1} MiB/s",
        megabytes / seconds
    )
}

fn bench_snapshot(out: &mut impl Write) -> std::io::Result<()> {
    let mut terminal = terminal();
    terminal.apply(&DaemonFrame::Output(colored_lines(
        CHUNK_BYTES * SNAPSHOT_FILL_CHUNKS,
    )));
    let stopwatch = Stopwatch::start();
    for _ in 0..SNAPSHOTS {
        black_box(terminal.snapshot());
    }
    let micros = stopwatch.elapsed_seconds() * MICROS_PER_SECOND / f64::from(SNAPSHOTS);
    writeln!(out, "snapshot {COLUMNS}x{LINES}: {micros:.1} us each")
}

fn main() -> std::io::Result<()> {
    let mut console = std::io::stdout().lock();
    bench_feed(&mut console)?;
    bench_snapshot(&mut console)
}
