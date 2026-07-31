// See the note in `differ.rs`: clippy's `allow-expect-in-tests` exemption only
// reaches `#[cfg(test)]` items and `#[test]` functions, not an integration-test
// helper. Cargo already builds this target with `--cfg test`, so this attribute
// changes nothing about what compiles.
#![cfg(test)]

//! The `--report` surface, exercised two ways.
//!
//! [`cli::run`] is driven in-process with in-memory streams, which is where the
//! output and exit-code assertions live. The real binary is then spawned once
//! per outcome, because the exit code is what CI actually gates on and an
//! in-process test cannot prove that `main` returns it.

mod support;

use std::ffi::OsString;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::atomic::{AtomicU32, Ordering};

use oracle::cli::{self, EXIT_CONVERGED, EXIT_DELTAS, EXIT_UNUSABLE};
use support::{EMPTY, REFERENCE, native};

static COUNTER: AtomicU32 = AtomicU32::new(0);

/// A scratch directory that cleans itself up.
struct Scratch(PathBuf);

impl Scratch {
    fn new() -> Self {
        let n = COUNTER.fetch_add(1, Ordering::Relaxed);
        let dir = std::env::temp_dir().join(format!("oracle-cli-{}-{n}", std::process::id()));
        std::fs::create_dir_all(&dir).expect("create scratch dir");
        Self(dir)
    }

    fn write(&self, name: &str, contents: &str) -> PathBuf {
        let path = self.0.join(name);
        std::fs::write(&path, contents).expect("write fixture");
        path
    }

    fn path(&self, name: &str) -> PathBuf {
        self.0.join(name)
    }
}

impl Drop for Scratch {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.0);
    }
}

/// What one in-process invocation produced.
struct Run {
    code: u8,
    out: String,
    err: String,
}

fn run(args: &[&str]) -> Run {
    let vector: Vec<OsString> = std::iter::once(OsString::from("oracle"))
        .chain(args.iter().map(OsString::from))
        .collect();
    let mut out = Vec::new();
    let mut err = Vec::new();
    let code = cli::run(vector, &mut out, &mut err);
    Run {
        code,
        out: String::from_utf8(out).expect("stdout is utf-8"),
        err: String::from_utf8(err).expect("stderr is utf-8"),
    }
}

fn report(reference: &Path, native_path: &Path, extra: &[&str]) -> Run {
    let mut args = vec![
        "--report",
        reference.to_str().expect("utf-8 path"),
        native_path.to_str().expect("utf-8 path"),
    ];
    args.extend_from_slice(extra);
    run(&args)
}

#[test]
fn converged_snapshots_exit_zero_and_say_which_matrix_cell() {
    let scratch = Scratch::new();
    let a = scratch.write("reference.json", REFERENCE);
    let b = scratch.write("native.json", REFERENCE);
    let result = report(&a, &b, &[]);
    assert_eq!(result.code, EXIT_CONVERGED);
    assert_eq!(
        result.out,
        "oracle: git-status-row · width=320 theme=dark content=overflow \
         flags=[hover, selected]\n\noracle: PASS — 0 deltas over 3 anchors compared\n"
    );
    assert!(result.err.is_empty());
}

#[test]
fn deltas_exit_one_and_print_the_ranked_list() {
    let scratch = Scratch::new();
    let a = scratch.write("reference.json", REFERENCE);
    let b = scratch.write("native.json", &native("\"x\": 8.0", "\"x\": 12.0"));
    let result = report(&a, &b, &[]);
    assert_eq!(result.code, EXIT_DELTAS);
    assert_eq!(
        result.out,
        "oracle: git-status-row · width=320 theme=dark content=overflow \
         flags=[hover, selected]\n\n\
         git-row-icon.bounds.x: 12.0, expected 8.0 (Δ +4.0, tol ±0.5)\n\n\
         oracle: FAIL — 1 delta over 3 anchors compared (1 geometry)\n"
    );
    assert!(result.err.is_empty());
}

#[test]
fn differing_anchor_counts_are_stated_in_the_header() {
    let scratch = Scratch::new();
    let a = scratch.write("reference.json", REFERENCE);
    let b = scratch.write(
        "native.json",
        &REFERENCE.replace(
            "    { \"id\": \"git-row-icon\",\n      \"bounds\": { \"x\": 8.0, \"y\": 5.0, \
             \"w\": 14.0, \"h\": 14.0 },\n      \"bg\": \"#00000000\",\n      \
             \"visible\": true },\n",
            "",
        ),
    );
    let result = report(&a, &b, &[]);
    assert_eq!(result.code, EXIT_DELTAS);
    assert!(
        result
            .out
            .contains("3 anchors in the reference, 2 in the native snapshot"),
        "{}",
        result.out
    );
}

#[test]
fn a_mismatched_matrix_cell_exits_two_not_one() {
    // The distinction matters: 1 means the native app is wrong, 2 means the
    // oracle could not tell you anything.
    let scratch = Scratch::new();
    let a = scratch.write("reference.json", REFERENCE);
    let b = scratch.write(
        "native.json",
        &native("\"theme\": \"dark\"", "\"theme\": \"light\""),
    );
    let result = report(&a, &b, &[]);
    assert_eq!(result.code, EXIT_UNUSABLE);
    assert!(result.err.contains("refusing to compare"), "{}", result.err);
    assert!(result.err.contains("state.theme"), "{}", result.err);
    assert!(result.out.is_empty());
}

#[test]
fn an_empty_comparison_is_refused_unless_asked_for() {
    let scratch = Scratch::new();
    let a = scratch.write("reference.json", EMPTY);
    let b = scratch.write("native.json", EMPTY);

    let refused = report(&a, &b, &[]);
    assert_eq!(refused.code, EXIT_UNUSABLE);
    assert!(
        refused.err.contains("refusing a comparison of 0 anchors"),
        "{}",
        refused.err
    );
    // The report is still printed, so the refusal is explicable.
    assert!(refused.out.contains("PASS — 0 deltas"), "{}", refused.out);

    let allowed = report(&a, &b, &["--allow-empty"]);
    assert_eq!(allowed.code, EXIT_CONVERGED);
    assert!(allowed.err.is_empty());
}

#[test]
fn unreadable_and_malformed_input_exit_two_and_name_the_file() {
    let scratch = Scratch::new();
    let good = scratch.write("reference.json", REFERENCE);
    let absent = scratch.path("does-not-exist.json");

    let missing_reference = report(&absent, &good, &[]);
    assert_eq!(missing_reference.code, EXIT_UNUSABLE);
    assert!(
        missing_reference.err.contains("cannot read"),
        "{}",
        missing_reference.err
    );
    assert!(
        missing_reference.err.contains("does-not-exist.json"),
        "{}",
        missing_reference.err
    );

    let missing_native = report(&good, &absent, &[]);
    assert_eq!(missing_native.code, EXIT_UNUSABLE);
    assert!(missing_native.err.contains("cannot read"));

    let junk = scratch.write("junk.json", "{ not json");
    let bad_reference = report(&junk, &good, &[]);
    assert_eq!(bad_reference.code, EXIT_UNUSABLE);
    assert!(
        bad_reference.err.contains("is not a v1 snapshot"),
        "{}",
        bad_reference.err
    );
    assert!(
        bad_reference.err.contains("junk.json"),
        "{}",
        bad_reference.err
    );

    let bad_native = report(&good, &junk, &[]);
    assert_eq!(bad_native.code, EXIT_UNUSABLE);
    assert!(bad_native.err.contains("is not a v1 snapshot"));

    let bad_colour = scratch.write(
        "bad-colour.json",
        &native("\"bg\": \"#1e2228ff\"", "\"bg\": \"#1e2228\""),
    );
    let result = report(&good, &bad_colour, &[]);
    assert_eq!(result.code, EXIT_UNUSABLE);
    assert!(
        result.err.contains("the alpha is not optional"),
        "{}",
        result.err
    );
}

#[test]
fn usage_errors_exit_two_and_print_the_usage() {
    let cases: &[(&str, &[&str], &str)] = &[
        ("no arguments at all", &[], "no mode given"),
        (
            "a mode with no paths",
            &["--report"],
            "exactly two snapshot paths",
        ),
        (
            "a mode with one path",
            &["--report", "a.json"],
            "exactly two snapshot paths",
        ),
        (
            "a mode with three paths",
            &["--report", "a.json", "b.json", "c.json"],
            "exactly two snapshot paths",
        ),
        (
            "an unknown option",
            &["--report", "--tolerance-bounds=4", "a.json", "b.json"],
            "unknown option `--tolerance-bounds=4`",
        ),
        ("paths but no mode", &["a.json", "b.json"], "no mode given"),
    ];
    for (what, args, needle) in cases {
        let result = run(args);
        assert_eq!(result.code, EXIT_UNUSABLE, "case: {what}");
        assert!(
            result.err.contains(needle),
            "case {what}: {} lacks {needle}",
            result.err
        );
        assert!(result.err.contains("USAGE:"), "case: {what}");
    }
}

#[test]
fn help_exits_zero_and_documents_the_exit_codes() {
    for flag in ["-h", "--help"] {
        let result = run(&[flag]);
        assert_eq!(result.code, EXIT_CONVERGED, "flag: {flag}");
        assert!(result.out.contains("EXIT CODES:"), "flag: {flag}");
        assert!(result.out.contains("--allow-empty"), "flag: {flag}");
        assert!(result.err.is_empty(), "flag: {flag}");
    }
    // `--help` wins wherever it appears.
    let result = run(&["--report", "a.json", "b.json", "--help"]);
    assert_eq!(result.code, EXIT_CONVERGED);
    assert!(result.out.contains("USAGE:"));
}

#[test]
fn a_bare_dash_is_a_path_not_an_option() {
    // So a caller can pipe in a file literally named `-` without the argument
    // parser guessing at a convention nobody asked for.
    let result = run(&["--report", "-", "-"]);
    assert_eq!(result.code, EXIT_UNUSABLE);
    assert!(result.err.contains("cannot read"), "{}", result.err);
}

#[test]
fn there_is_no_command_line_tolerance_override() {
    // ANCHORS.md §5 requires a loosened tolerance to land in its own commit
    // with a justification. A flag would let it happen invisibly in CI config,
    // so every spelling of one must be rejected rather than quietly honoured.
    for flag in [
        "--tolerance",
        "--tolerance-bounds",
        "--bounds-px",
        "--loose",
        "--fail-under",
    ] {
        let result = run(&["--report", flag, "a.json", "b.json"]);
        assert_eq!(result.code, EXIT_UNUSABLE, "flag: {flag}");
        assert!(
            result.err.contains("unknown option"),
            "flag {flag}: {}",
            result.err
        );
    }
}

// ---------------------------------------------------------------------------
// The real binary. In-process tests cannot prove `main` propagates the code.
// ---------------------------------------------------------------------------

fn spawn(args: &[&str]) -> (i32, String, String) {
    let output = Command::new(env!("CARGO_BIN_EXE_oracle"))
        .args(args)
        .output()
        .expect("the oracle binary must run");
    (
        output
            .status
            .code()
            .expect("the process must exit normally"),
        String::from_utf8(output.stdout).expect("stdout is utf-8"),
        String::from_utf8(output.stderr).expect("stderr is utf-8"),
    )
}

#[test]
fn the_binary_returns_each_exit_code_for_real() {
    let scratch = Scratch::new();
    let reference = scratch.write("reference.json", REFERENCE);
    let same = scratch.write("same.json", REFERENCE);
    let moved = scratch.write("moved.json", &native("\"x\": 8.0", "\"x\": 12.0"));
    let other_cell = scratch.write(
        "other-cell.json",
        &native("\"theme\": \"dark\"", "\"theme\": \"light\""),
    );

    let r = reference.to_str().expect("utf-8 path");

    let (code, out, err) = spawn(&["--report", r, same.to_str().expect("utf-8")]);
    assert_eq!(code, i32::from(EXIT_CONVERGED), "stderr: {err}");
    assert!(
        out.contains("PASS — 0 deltas over 3 anchors compared"),
        "{out}"
    );

    let (code, out, _) = spawn(&["--report", r, moved.to_str().expect("utf-8")]);
    assert_eq!(code, i32::from(EXIT_DELTAS));
    assert!(
        out.contains("git-row-icon.bounds.x: 12.0, expected 8.0 (Δ +4.0, tol ±0.5)"),
        "{out}"
    );

    let (code, _, err) = spawn(&["--report", r, other_cell.to_str().expect("utf-8")]);
    assert_eq!(code, i32::from(EXIT_UNUSABLE));
    assert!(err.contains("refusing to compare"), "{err}");

    let (code, out, _) = spawn(&["--help"]);
    assert_eq!(code, i32::from(EXIT_CONVERGED));
    assert!(out.contains("USAGE:"), "{out}");

    let (code, _, err) = spawn(&[]);
    assert_eq!(code, i32::from(EXIT_UNUSABLE));
    assert!(err.contains("no mode given"), "{err}");
}
