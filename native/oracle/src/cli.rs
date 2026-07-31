//! The `--report` entry point (§12).
//!
//! `main.rs` is two lines; everything the CLI actually does lives here so the
//! ≥98% coverage gate can reach it.
//!
//! # Exit codes
//!
//! CI gates on these, so they are a contract of their own and the three
//! outcomes are deliberately distinguishable:
//!
//! | Code | Meaning |
//! |---|---|
//! | 0 | Every anchor converged. |
//! | 1 | At least one delta exceeded tolerance. **The gate failed.** |
//! | 2 | The comparison could not be made: bad usage, unreadable or malformed input, mismatched matrix cells, or an empty comparison. |
//!
//! 1 and 2 are separated because they mean opposite things to a human. A `1`
//! says the native app is wrong. A `2` says *the oracle could not tell you
//! anything*, and treating that as a pass — or as a failure of the app — is how
//! a broken harness gets mistaken for a converged one.
//!
//! # There is deliberately no `--tolerance-*` flag
//!
//! ANCHORS.md §5 requires a loosened tolerance to land in its own commit,
//! stating the measurement that justified it and the defect class it can no
//! longer catch. A command-line override would let the same loosening happen
//! invisibly inside a CI config, which is exactly the failure §5 is written to
//! prevent. [`Tolerances`] is overridable as a library type, where the change
//! is a reviewable diff.

use std::ffi::OsString;
use std::fmt::Write as _;
use std::io::Write;
use std::path::{Path, PathBuf};

use crate::diff::{Forgiven, Forgiveness, Report, diff};
use crate::snapshot::Snapshot;
use crate::tolerance::Tolerances;

/// Exit code: every anchor converged.
pub const EXIT_CONVERGED: u8 = 0;
/// Exit code: at least one delta exceeded tolerance.
pub const EXIT_DELTAS: u8 = 1;
/// Exit code: the comparison could not be made at all.
pub const EXIT_UNUSABLE: u8 = 2;

const USAGE: &str = "\
oracle — the parity differ (spec §8.1). Two v1 snapshots in, ranked deltas out.

USAGE:
    oracle --report <reference.json> <native.json> [--allow-empty]

ARGUMENTS:
    <reference.json>  snapshot from the React app — the `expected` side
    <native.json>     snapshot from the GPUI app — the `actual` side

OPTIONS:
    --allow-empty     do not fail when the two snapshots share no anchors
    -h, --help        print this

EXIT CODES:
    0  converged
    1  deltas exceeded tolerance — the gate failed
    2  no comparison was possible (usage, IO, malformed input, mismatched
       §8.3 matrix cells, or an empty comparison)

Both snapshots must be the same `schema`, `surface` and `state`. Comparing
across states is refused, not reported as a delta: a delta between different
states is meaningless (ANCHORS.md §2).";

/// What the argument vector asked for.
#[derive(Debug, Clone, PartialEq, Eq)]
enum Invocation {
    Help,
    Report {
        reference: PathBuf,
        native: PathBuf,
        allow_empty: bool,
    },
}

/// Run the CLI. Returns the process exit code.
///
/// `args` is the full argument vector including `argv[0]`, which is skipped.
/// Output goes to `out`, diagnostics to `err`; both are injected so the whole
/// CLI is testable without a process.
pub fn run<A, O, E>(args: A, out: &mut O, err: &mut E) -> u8
where
    A: IntoIterator<Item = OsString>,
    O: Write,
    E: Write,
{
    let invocation = match parse_args(args) {
        Ok(invocation) => invocation,
        Err(message) => {
            let _ = writeln!(err, "oracle: {message}\n\n{USAGE}");
            return EXIT_UNUSABLE;
        }
    };

    let Invocation::Report {
        reference,
        native,
        allow_empty,
    } = invocation
    else {
        let _ = writeln!(out, "{USAGE}");
        return EXIT_CONVERGED;
    };

    let reference_text = match read(&reference) {
        Ok(text) => text,
        Err(message) => {
            let _ = writeln!(err, "oracle: {message}");
            return EXIT_UNUSABLE;
        }
    };
    let native_text = match read(&native) {
        Ok(text) => text,
        Err(message) => {
            let _ = writeln!(err, "oracle: {message}");
            return EXIT_UNUSABLE;
        }
    };

    let expected = match Snapshot::from_json(&reference_text) {
        Ok(snapshot) => snapshot,
        Err(e) => {
            let _ = writeln!(
                err,
                "oracle: {} is not a v1 snapshot: {e}",
                reference.display()
            );
            return EXIT_UNUSABLE;
        }
    };
    let actual = match Snapshot::from_json(&native_text) {
        Ok(snapshot) => snapshot,
        Err(e) => {
            let _ = writeln!(
                err,
                "oracle: {} is not a v1 snapshot: {e}",
                native.display()
            );
            return EXIT_UNUSABLE;
        }
    };

    let report = match diff(&expected, &actual, &Tolerances::V1) {
        Ok(report) => report,
        Err(mismatch) => {
            let _ = writeln!(err, "oracle: {mismatch}");
            return EXIT_UNUSABLE;
        }
    };

    let _ = write!(out, "{}", render(&report));

    if report.anchors_compared == 0 && !allow_empty {
        let _ = writeln!(
            err,
            "oracle: refusing a comparison of 0 anchors. \"0 deltas over 0 anchors\" is a pass \
             on paper and evidence of nothing — the reference snapshot carried {} anchor(s) and \
             the native one {}, and they share no ids. Pass --allow-empty if that is genuinely \
             what you meant.",
            report.reference_anchors, report.native_anchors
        );
        return EXIT_UNUSABLE;
    }

    if report.is_clean() {
        EXIT_CONVERGED
    } else {
        EXIT_DELTAS
    }
}

/// The whole report as text: context header, ranked deltas, verdict.
#[must_use]
pub fn render(report: &Report) -> String {
    let mut s = format!("oracle: {} · {}\n", report.surface, report.state);
    if report.reference_anchors != report.native_anchors {
        let _ = writeln!(
            s,
            "        {} anchors in the reference, {} in the native snapshot",
            report.reference_anchors, report.native_anchors
        );
    }
    if !report.deltas.is_empty() {
        s.push('\n');
        for delta in &report.deltas {
            let _ = writeln!(s, "{delta}");
        }
    }
    render_content_sizing(&mut s, report);
    render_line_sizing(&mut s, report);
    s.push('\n');
    let _ = writeln!(s, "oracle: {}", report.summary());
    s
}

/// The v1.5 content-sizing section: what was declared, how much slack it
/// bought, and every comparison that only passed because of it.
///
/// Printed whenever a **v1.5** rule forgave something, and only then — v1.6's
/// line-sizing has its own section below. ANCHORS.md §5 calls a
/// silent widening the cheapest way to make a gate pass while telling you
/// nothing — so the allowance is never applied without this section appearing,
/// and the section names the anchors it came from so a reader can check the
/// arithmetic against the snapshot rather than take it on trust.
///
/// A non-empty `forgiven` implies a non-empty contributor list, so the header
/// always has something to name: both forgivenesses require a declared anchor
/// whose reference width carried a fraction, and that is exactly what a
/// contributor is. An integral declared width forgives nothing.
fn render_content_sizing(s: &mut String, report: &Report) {
    let forgiven = by_rule(report, false);
    if forgiven.is_empty() {
        return;
    }
    let sizing = &report.content_sizing;
    let contributors: Vec<String> = sizing
        .contributors
        .iter()
        .map(|c| {
            format!(
                "{} +{}",
                c.anchor,
                crate::delta::px(crate::diff::round3(c.excess_px))
            )
        })
        .collect();
    let _ = write!(
        s,
        "\ncontent-sizing (ANCHORS.md v1.5): {} anchor(s) declared content_sized, \
         Σ ceil excess {} px",
        sizing.contributors.len(),
        crate::delta::px(crate::diff::round3(sizing.excess_px))
    );
    let _ = writeln!(s, " — {}", contributors.join(", "));
    for entry in forgiven {
        let _ = writeln!(s, "  {entry}");
    }
}

/// The v1.6 line-sizing section: every comparison that only passed because
/// `bounds.h` was measured against the reference's `font.line_height`.
///
/// A section of its own rather than a line in the one above. The two rules
/// forgive different things for different reasons — v1.5's a one-directional
/// `ceil` on the inline axis, v1.6's two engines quantising the same line box —
/// and a reader who has learnt one must not be able to skim the other as more
/// of it.
fn render_line_sizing(s: &mut String, report: &Report) {
    let forgiven = by_rule(report, true);
    if forgiven.is_empty() {
        return;
    }
    let _ = writeln!(
        s,
        "\nline-sizing (ANCHORS.md v1.6): {} comparison(s) took bounds.h against the reference's \
         font.line_height rather than its box height",
        forgiven.len(),
    );
    for entry in forgiven {
        let _ = writeln!(s, "  {entry}");
    }
}

/// The forgiven comparisons under one of the two rules — v1.6's line-sizing
/// when `line_sized`, v1.5's content-sizing otherwise.
fn by_rule(report: &Report, line_sized: bool) -> Vec<&Forgiven> {
    report
        .forgiven
        .iter()
        .filter(|forgiven| {
            matches!(forgiven.reason, Forgiveness::LineHeight { .. }) == line_sized
        })
        .collect()
}

fn read(path: &Path) -> Result<String, String> {
    std::fs::read_to_string(path).map_err(|e| format!("cannot read {}: {e}", path.display()))
}

fn parse_args<A>(args: A) -> Result<Invocation, String>
where
    A: IntoIterator<Item = OsString>,
{
    let mut mode_seen = false;
    let mut allow_empty = false;
    let mut positional: Vec<PathBuf> = Vec::new();

    for arg in args.into_iter().skip(1) {
        match arg.to_str() {
            Some("-h" | "--help") => return Ok(Invocation::Help),
            Some("--report") => mode_seen = true,
            Some("--allow-empty") => allow_empty = true,
            Some(flag) if flag.starts_with('-') && flag != "-" => {
                return Err(format!("unknown option `{flag}`"));
            }
            _ => positional.push(PathBuf::from(arg)),
        }
    }

    if !mode_seen {
        return Err("no mode given; --report is the only one".to_owned());
    }
    match positional.as_slice() {
        [reference, native] => Ok(Invocation::Report {
            reference: reference.clone(),
            native: native.clone(),
            allow_empty,
        }),
        other => Err(format!(
            "--report takes exactly two snapshot paths (reference, then native); got {}",
            other.len()
        )),
    }
}
