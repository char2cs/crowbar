#![forbid(unsafe_code)]

//! CLI entry point for the parity differ. Invoked as
//! `cargo run -p oracle -- --report <reference.json> <native.json>` (§12).
//!
//! Deliberately one expression: everything the CLI does lives in
//! [`oracle::cli::run`], which takes its streams as arguments so all of it is
//! reachable from a test. Logic in `main` would be a hole in the §12 coverage
//! gate, since a unit test never runs it.

use std::process::ExitCode;

fn main() -> ExitCode {
    ExitCode::from(oracle::cli::run(
        std::env::args_os(),
        &mut std::io::stdout(),
        &mut std::io::stderr(),
    ))
}
