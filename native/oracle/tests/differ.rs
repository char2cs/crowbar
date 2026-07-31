// `clippy.toml` exempts test code from the §4.3 rule-4 denials via
// `allow-expect-in-tests`, but clippy only recognises that exemption inside a
// `#[cfg(test)]` item or a `#[test]` function — an integration-test *helper* is
// neither. This crate-level attribute makes the whole file `cfg(test)`, which
// it already is (cargo builds test targets with `--cfg test`), so it changes
// nothing about what compiles and everything about whether a shared helper may
// say `expect` in a test.
#![cfg(test)]

//! End-to-end behaviour of the differ, driven from JSON exactly as the
//! orchestrator will drive it.
//!
//! Table-driven throughout. Each case names one field, mutates exactly that
//! field in the native snapshot, and asserts the **complete ranked delta list**
//! rather than "some delta happened" — so a case that produces an extra delta,
//! or the same delta with the wrong wording or in the wrong position, fails.

mod support;

use oracle::{Snapshot, Tolerances, diff};
use support::{BOX_AND_TEXT, EMPTY, REFERENCE, native, once};

/// Parse both sides and render the ranked deltas as strings.
fn deltas_of(native_json: &str) -> Vec<String> {
    deltas_between(REFERENCE, native_json)
}

/// The same, for a pair where the *reference* side is mutated too.
fn deltas_between(reference_json: &str, native_json: &str) -> Vec<String> {
    let expected = Snapshot::from_json(reference_json).expect("reference fixture must load");
    let actual = Snapshot::from_json(native_json).expect("native fixture must load");
    diff(&expected, &actual, &Tolerances::V1)
        .expect("same matrix cell")
        .deltas
        .iter()
        .map(ToString::to_string)
        .collect()
}

/// One mutation of the reference, and the exact ranked deltas it must produce.
struct Case {
    what: &'static str,
    from: &'static str,
    to: &'static str,
    want: &'static [&'static str],
}

fn run(cases: &[Case]) {
    for case in cases {
        let got = deltas_of(&native(case.from, case.to));
        let want: Vec<String> = case.want.iter().map(|s| (*s).to_owned()).collect();
        assert_eq!(got, want, "case: {}", case.what);
    }
}

#[test]
fn identical_snapshots_produce_no_deltas() {
    let snapshot = Snapshot::from_json(REFERENCE).expect("valid");
    let report = diff(&snapshot, &snapshot, &Tolerances::V1).expect("same cell");
    assert!(report.is_clean());
    assert_eq!(report.anchors_compared, 3);
    assert_eq!(report.reference_anchors, 3);
    assert_eq!(report.native_anchors, 3);
    assert_eq!(report.summary(), "PASS — 0 deltas over 3 anchors compared");
    assert!(report.by_class().is_empty());
    assert_eq!(report.surface, "git-status-row");
}

#[test]
fn anchor_order_is_irrelevant_because_matching_is_by_id() {
    // §1: the two element trees are not isomorphic, so nothing may depend on
    // the order the extractor happened to walk them in.
    let reordered = REFERENCE
        .replace("\"id\": \"git-row-item\"", "\"id\": \"ZZZ\"")
        .replace("\"id\": \"git-row-name\"", "\"id\": \"git-row-item\"")
        .replace("\"id\": \"ZZZ\"", "\"id\": \"git-row-name\"")
        .replace("\"root\": \"git-row-item\"", "\"root\": \"git-row-name\"");
    // The first anchor is now `git-row-name` at the origin and the third is
    // `git-row-item`; every id still exists, so ids that match still compare.
    let expected = Snapshot::from_json(REFERENCE).expect("valid");
    let actual = Snapshot::from_json(&reordered).expect("valid");
    let report = diff(&expected, &actual, &Tolerances::V1).expect("same cell");
    assert_eq!(report.anchors_compared, 3);
    // The two swapped anchors genuinely have different geometry now, so the
    // point of the assertion is that nothing is reported as *missing*.
    assert!(
        !report
            .deltas
            .iter()
            .any(|d| d.class == oracle::Class::AnchorPresence),
        "id matching must not depend on position: {:?}",
        report.deltas
    );
}

#[test]
fn geometry_deltas_report_magnitude_and_the_tolerance_they_broke() {
    run(&[
        Case {
            what: "bounds.x past tolerance",
            from: "\"x\": 8.0",
            to: "\"x\": 12.0",
            want: &["git-row-icon.bounds.x: 12.0, expected 8.0 (Δ +4.0, tol ±0.5)"],
        },
        Case {
            what: "bounds.y past tolerance, negative",
            from: "\"y\": 5.0",
            to: "\"y\": 1.0",
            want: &["git-row-icon.bounds.y: 1.0, expected 5.0 (Δ -4.0, tol ±0.5)"],
        },
        Case {
            what: "bounds.w",
            from: "\"w\": 14.0",
            to: "\"w\": 16.0",
            want: &["git-row-icon.bounds.w: 16.0, expected 14.0 (Δ +2.0, tol ±0.5)"],
        },
        Case {
            what: "bounds.h",
            from: "\"h\": 14.0",
            to: "\"h\": 15.0",
            want: &["git-row-icon.bounds.h: 15.0, expected 14.0 (Δ +1.0, tol ±0.5)"],
        },
        Case {
            what: "text_width",
            from: "\"text_width\": 186.5",
            to: "\"text_width\": 190.0",
            want: &["git-row-name.text_width: 190.0, expected 186.5 (Δ +3.5, tol ±1.0)"],
        },
        Case {
            what: "radius",
            from: "\"radius\": 2.0",
            to: "\"radius\": 6.0",
            want: &["git-row-item.radius: 6.0, expected 2.0 (Δ +4.0, tol ±0.5)"],
        },
    ]);
}

/// §5, v1.1 ruling 1: `border.w` is **exact**. ±0.5 on a 1.0px border is a 50%
/// error and plainly visible, so there is no tolerance to be at the boundary of
/// — which is why these cases are here and not in the tolerance tests.
#[test]
fn border_width_is_exact_not_tolerated() {
    run(&[
        Case {
            what: "half a pixel wider is already a defect",
            from: "\"border\": { \"w\": 1.0",
            to: "\"border\": { \"w\": 1.5",
            want: &["git-row-item.border.w: 1.5, expected 1.0 (Δ +0.5, exact)"],
        },
        Case {
            what: "half a pixel narrower likewise",
            from: "\"border\": { \"w\": 1.0",
            to: "\"border\": { \"w\": 0.5",
            want: &["git-row-item.border.w: 0.5, expected 1.0 (Δ -0.5, exact)"],
        },
        Case {
            // The Δ reads `+0.063` — three decimals, per v1.2 — while the value
            // itself is printed in full. Nothing was forgiven: exact means
            // exact, and this is still a delta.
            what: "and so is a sixteenth",
            from: "\"border\": { \"w\": 1.0",
            to: "\"border\": { \"w\": 1.0625",
            want: &["git-row-item.border.w: 1.0625, expected 1.0 (Δ +0.063, exact)"],
        },
    ]);
}

/// v1.3 ruling 2. A zero-width border paints nothing, and neither engine
/// reports a meaningful colour for one: the DOM returns the element's inherited
/// *text* colour. On the first real gate run this produced eight deltas across
/// eight anchors, every one of them about paint that never happened.
#[test]
fn border_colour_is_only_compared_when_the_border_is_painted() {
    let zero_width = "\"border\": { \"w\": 0.0, \"color\": \"#f5f5f5ff\" }";

    // Both sides zero-width, wildly different junk colours: not a delta.
    let reference = once(
        REFERENCE,
        "\"border\": { \"w\": 1.0, \"color\": \"#00000000\" }",
        zero_width,
    );
    let native_side = once(
        &reference,
        zero_width,
        "\"border\": { \"w\": 0.0, \"color\": \"#00000000\" }",
    );
    assert!(
        deltas_between(&reference, &native_side).is_empty(),
        "a colour on a border nobody paints is not a finding: {:?}",
        deltas_between(&reference, &native_side)
    );

    // One side zero-width: the *width* is the delta, and it is not joined by a
    // colour delta comparing a real colour against junk.
    let widened = once(
        &reference,
        zero_width,
        "\"border\": { \"w\": 1.0, \"color\": \"#00000000\" }",
    );
    assert_eq!(
        deltas_between(&reference, &widened),
        vec!["git-row-item.border.w: 1.0, expected 0.0 (Δ +1.0, exact)"]
    );

    // And with both sides painting, the colour is compared exactly as before —
    // the rule narrows what is checked, it does not switch it off.
    assert_eq!(
        deltas_of(&native(
            "\"color\": \"#00000000\"",
            "\"color\": \"#ff000000\""
        )),
        vec!["git-row-item.border.color: #ff000000, expected #00000000 (Δ r +255, rgb is exact)"]
    );
}

#[test]
fn a_value_exactly_at_the_tolerance_passes() {
    // Values are exactly representable in binary floating point, so these
    // cases test the comparison rather than the arithmetic.
    let at_boundary: &[(&str, &str, &str)] = &[
        ("bounds +0.5", "\"x\": 8.0", "\"x\": 8.5"),
        ("bounds -0.5", "\"x\": 8.0", "\"x\": 7.5"),
        (
            "text_width +1.0",
            "\"text_width\": 186.5",
            "\"text_width\": 187.5",
        ),
        (
            "text_width -1.0",
            "\"text_width\": 186.5",
            "\"text_width\": 185.5",
        ),
        ("font.size +0.5", "\"size\": 13.0", "\"size\": 13.5"),
        ("font.size -0.5", "\"size\": 13.0", "\"size\": 12.5"),
        (
            "line_height +0.5",
            "\"line_height\": 17.5",
            "\"line_height\": 18.0",
        ),
        (
            "line_height -0.5",
            "\"line_height\": 17.5",
            "\"line_height\": 17.0",
        ),
        ("radius +0.5", "\"radius\": 2.0", "\"radius\": 2.5"),
        (
            "alpha +1/255",
            "\"bg\": \"#1e2228ff\"",
            "\"bg\": \"#1e2228fe\"",
        ),
    ];
    for (what, from, to) in at_boundary {
        assert!(
            deltas_of(&native(from, to)).is_empty(),
            "exactly at tolerance must pass: {what}"
        );
    }
}

#[test]
fn a_value_just_past_the_tolerance_fails_in_both_directions() {
    let past_boundary: &[(&str, &str, &str, &str)] = &[
        (
            "bounds +0.625",
            "\"x\": 8.0",
            "\"x\": 8.625",
            "git-row-icon.bounds.x: 8.625, expected 8.0 (Δ +0.625, tol ±0.5)",
        ),
        (
            "bounds -0.625",
            "\"x\": 8.0",
            "\"x\": 7.375",
            "git-row-icon.bounds.x: 7.375, expected 8.0 (Δ -0.625, tol ±0.5)",
        ),
        (
            "text_width +1.25",
            "\"text_width\": 186.5",
            "\"text_width\": 187.75",
            "git-row-name.text_width: 187.75, expected 186.5 (Δ +1.25, tol ±1.0)",
        ),
        (
            "text_width -1.25",
            "\"text_width\": 186.5",
            "\"text_width\": 185.25",
            "git-row-name.text_width: 185.25, expected 186.5 (Δ -1.25, tol ±1.0)",
        ),
        (
            "font.size +0.75",
            "\"size\": 13.0",
            "\"size\": 13.75",
            "git-row-name.font.size: 13.75, expected 13.0 (Δ +0.75, tol ±0.5)",
        ),
        (
            "line_height -0.75",
            "\"line_height\": 17.5",
            "\"line_height\": 16.75",
            "git-row-name.font.line_height: 16.75, expected 17.5 (Δ -0.75, tol ±0.5)",
        ),
        (
            "radius +0.75",
            "\"radius\": 2.0",
            "\"radius\": 2.75",
            "git-row-item.radius: 2.75, expected 2.0 (Δ +0.75, tol ±0.5)",
        ),
        (
            "alpha +2/255",
            "\"bg\": \"#1e2228ff\"",
            "\"bg\": \"#1e2228fd\"",
            "git-row-item.bg: #1e2228fd, expected #1e2228ff (Δ a -2, tol ±1/255)",
        ),
    ];
    for (what, from, to, want) in past_boundary {
        assert_eq!(
            deltas_of(&native(from, to)),
            vec![(*want).to_owned()],
            "just past tolerance must fail: {what}"
        );
    }
}

#[test]
fn colour_is_rgb_exact_and_alpha_tolerant() {
    run(&[
        Case {
            what: "one unit of red is already a defect",
            from: "\"bg\": \"#1e2228ff\"",
            to: "\"bg\": \"#1f2228ff\"",
            want: &["git-row-item.bg: #1f2228ff, expected #1e2228ff (Δ r +1, rgb is exact)"],
        },
        Case {
            what: "foreground colour",
            from: "\"fg\": \"#c8ccd4ff\"",
            to: "\"fg\": \"#8a90a0ff\"",
            want: &["git-row-name.fg: #8a90a0ff, expected #c8ccd4ff (Δ r -62, rgb is exact)"],
        },
        Case {
            what: "border colour",
            from: "\"color\": \"#00000000\"",
            to: "\"color\": \"#ff000000\"",
            want: &[
                "git-row-item.border.color: #ff000000, expected #00000000 (Δ r +255, rgb is exact)",
            ],
        },
        Case {
            what: "rgb wrongness outranks its own alpha wrongness in the message",
            from: "\"bg\": \"#1e2228ff\"",
            to: "\"bg\": \"#1e2230f0\"",
            want: &["git-row-item.bg: #1e2230f0, expected #1e2228ff (Δ b +8, rgb is exact)"],
        },
    ]);
}

#[test]
fn exact_fields_report_as_exact() {
    run(&[
        Case {
            what: "visible",
            from: "\"bg\": \"#00000000\",\n      \"visible\": true },",
            to: "\"bg\": \"#00000000\",\n      \"visible\": false },",
            want: &["git-row-icon.visible: false, expected true (exact)"],
        },
        Case {
            what: "text",
            from: "\"text\": \"resolve-terminal-connection.ts\"",
            to: "\"text\": \"resolve-terminal-connection.tsx\"",
            want: &["git-row-name.text: \"resolve-terminal-connection.tsx\", \
                 expected \"resolve-terminal-connection.ts\" (exact)"],
        },
        Case {
            what: "clipped",
            from: "\"clipped\": true",
            to: "\"clipped\": false",
            want: &["git-row-name.clipped: false, expected true (exact)"],
        },
        Case {
            what: "font weight",
            from: "\"weight\": 500",
            to: "\"weight\": 400",
            want: &["git-row-name.font.weight: 400, expected 500 (Δ -100, exact)"],
        },
        Case {
            what: "font family",
            from: "\"family\": \"Inter\"",
            to: "\"family\": \"SF Pro Text\"",
            want: &["git-row-name.font.family: \"SF Pro Text\", expected \"Inter\" (exact)"],
        },
    ]);
}

#[test]
fn an_anchor_on_one_side_only_is_its_own_kind_of_delta() {
    let without_icon = REFERENCE.replace(
        "    { \"id\": \"git-row-icon\",\n      \"bounds\": { \"x\": 8.0, \"y\": 5.0, \
         \"w\": 14.0, \"h\": 14.0 },\n      \"bg\": \"#00000000\",\n      \"visible\": true },\n",
        "",
    );
    assert_eq!(
        deltas_of(&without_icon),
        vec![
            "git-row-icon: anchor missing from the native snapshot (present in the reference)"
                .to_owned()
        ]
    );

    let with_extra = native(
        "    { \"id\": \"git-row-icon\",",
        "    { \"id\": \"git-row-ghost\",\n      \"bounds\": { \"x\": 0.0, \"y\": 0.0, \
         \"w\": 1.0, \"h\": 1.0 },\n      \"bg\": \"#00000000\", \"visible\": true },\n\
         \n    { \"id\": \"git-row-icon\",",
    );
    assert_eq!(
        deltas_of(&with_extra),
        vec![
            "git-row-ghost: anchor present in the native snapshot but not in the reference"
                .to_owned()
        ]
    );
}

#[test]
fn a_field_emitted_by_only_one_extractor_is_a_contract_defect() {
    // ANCHORS.md's opening rule: a field one side emits and the other does not
    // "looks like coverage and is not". It must be visible, and it must not be
    // mistaken for a value delta.
    run(&[
        Case {
            what: "radius dropped by the native side",
            from: "\"radius\": 2.0,\n      ",
            to: "",
            want: &[
                "git-row-item.radius: not emitted by the native extractor, expected 2.0 \
                 (field-presence mismatch, not a value delta)",
            ],
        },
        Case {
            what: "border dropped by the native side",
            from: ",\n      \"border\": { \"w\": 1.0, \"color\": \"#00000000\" }",
            to: "",
            want: &[
                "git-row-item.border: not emitted by the native extractor, expected \
                 { w: 1.0, color: #00000000 } (field-presence mismatch, not a value delta)",
            ],
        },
        Case {
            what: "font dropped by the native side",
            from: "\"font\": { \"size\": 13.0, \"weight\": 500, \"family\": \"Inter\",\n                \"line_height\": 17.5 },\n      ",
            to: "",
            want: &[
                "git-row-name.font: not emitted by the native extractor, expected \
                 { size: 13.0, weight: 500, family: \"Inter\", line_height: 17.5 } \
                 (field-presence mismatch, not a value delta)",
            ],
        },
        Case {
            what: "text group dropped by the native side",
            from: "\"text\": \"resolve-terminal-connection.ts\",\n      \"text_width\": 186.5,\n      \"clipped\": true,\n      ",
            to: "",
            want: &[
                "git-row-name.clipped: not emitted by the native extractor, expected true \
                 (field-presence mismatch, not a value delta)",
                "git-row-name.text: not emitted by the native extractor, expected \
                 \"resolve-terminal-connection.ts\" (field-presence mismatch, not a value delta)",
                "git-row-name.text_width: not emitted by the native extractor, expected 186.5 \
                 (field-presence mismatch, not a value delta)",
            ],
        },
        Case {
            what: "fg emitted only by the native side",
            from: "\"bg\": \"#00000000\",\n      \"visible\": true },",
            to: "\"bg\": \"#00000000\",\n      \"fg\": \"#ff0000ff\",\n      \"visible\": true },",
            want: &[
                "git-row-icon.fg: emitted by the native extractor as #ff0000ff, absent from the \
                 reference (field-presence mismatch, not a value delta)",
            ],
        },
    ]);
}

#[test]
fn ranking_puts_the_biggest_fact_at_the_top() {
    // One mutation of every severity class at once, plus two geometry deltas of
    // different magnitudes, asserted as a complete ordered list.
    let mutated = REFERENCE
        // Drop the icon anchor entirely.
        .replace(
            "    { \"id\": \"git-row-icon\",\n      \"bounds\": { \"x\": 8.0, \"y\": 5.0, \
             \"w\": 14.0, \"h\": 14.0 },\n      \"bg\": \"#00000000\",\n      \"visible\": true },\n",
            "",
        )
        // Field presence: the native side stops emitting `radius`.
        .replace("\"radius\": 2.0,\n      ", "")
        // Visibility.
        .replace(
            "\"line_height\": 17.5 },\n      \"visible\": true }",
            "\"line_height\": 17.5 },\n      \"visible\": false }",
        )
        // Text content.
        .replace("resolve-terminal-connection.ts", "resolve-terminal-connection.tsx")
        // Truncation.
        .replace("\"clipped\": true", "\"clipped\": false")
        // Colour.
        .replace("\"bg\": \"#1e2228ff\"", "\"bg\": \"#1e2230ff\"")
        // Geometry, two magnitudes.
        .replace("\"w\": 320.0", "\"w\": 360.0")
        .replace("\"y\": 4.0", "\"y\": 6.0")
        // Typography.
        .replace("\"weight\": 500", "\"weight\": 400");

    assert_eq!(
        deltas_of(&mutated),
        vec![
            "git-row-icon: anchor missing from the native snapshot (present in the reference)",
            "git-row-item.radius: not emitted by the native extractor, expected 2.0 \
             (field-presence mismatch, not a value delta)",
            "git-row-name.visible: false, expected true (exact)",
            "git-row-name.text: \"resolve-terminal-connection.tsx\", \
             expected \"resolve-terminal-connection.ts\" (exact)",
            "git-row-name.clipped: false, expected true (exact)",
            "git-row-item.bg: #1e2230ff, expected #1e2228ff (Δ b +8, rgb is exact)",
            "git-row-item.bounds.w: 360.0, expected 320.0 (Δ +40.0, tol ±0.5)",
            "git-row-name.bounds.y: 6.0, expected 4.0 (Δ +2.0, tol ±0.5)",
            "git-row-name.font.weight: 400, expected 500 (Δ -100, exact)",
        ]
    );
}

/// v1.4. The box fields and the text group are independent; an anchor may carry
/// both, and every field of both must still be compared.
#[test]
fn an_anchor_can_be_both_a_painted_box_and_a_text_run() {
    let snapshot = Snapshot::from_json(BOX_AND_TEXT).expect("valid");
    let report = diff(&snapshot, &snapshot, &Tolerances::V1).expect("same cell");
    assert!(report.is_clean());
    assert_eq!(report.anchors_compared, 1);

    // One mutation in each half of the anchor, asserted together: if either half
    // were being skipped because the other was present, one of these would be
    // missing.
    let mutated = once(BOX_AND_TEXT, "\"radius\": 4.0", "\"radius\": 9.0");
    let mutated = once(&mutated, "\"uncommitted\"", "\"staged\"");
    assert_eq!(
        deltas_between(BOX_AND_TEXT, &mutated),
        vec![
            "git-row-badge.text: \"staged\", expected \"uncommitted\" (exact)",
            "git-row-badge.radius: 9.0, expected 4.0 (Δ +5.0, tol ±0.5)",
        ]
    );

    // And the box's own border colour is compared, because this box paints one.
    assert_eq!(
        deltas_between(
            BOX_AND_TEXT,
            &once(BOX_AND_TEXT, "\"#fe9a0080\"", "\"#fe9a0180\"")
        ),
        vec!["git-row-badge.border.color: #fe9a0180, expected #fe9a0080 (Δ b +1, rgb is exact)"]
    );
}

/// v1.2 ruling 3 fixes the snapshot precision at three decimals. The printed
/// **difference** honours that; the comparison does not round at all.
#[test]
fn the_printed_magnitude_is_rounded_to_the_contracts_three_decimals() {
    // 476.49 - 424.047 is -52.442999999999984 in binary floating point. Twelve
    // digits of representation noise is noise a reader has to learn to skip.
    let reference = once(REFERENCE, "\"text_width\": 186.5", "\"text_width\": 476.49");
    let actual = once(
        &reference,
        "\"text_width\": 476.49",
        "\"text_width\": 424.047",
    );
    assert_eq!(
        deltas_between(&reference, &actual),
        vec!["git-row-name.text_width: 424.047, expected 476.49 (Δ -52.443, tol ±1.0)"]
    );

    // The values themselves are printed verbatim, so an extractor emitting more
    // precision than the contract allows stays visible rather than tidied away.
    let over_precise = once(REFERENCE, "\"x\": 8.0", "\"x\": 12.00048828125");
    assert_eq!(
        deltas_of(&over_precise),
        vec!["git-row-icon.bounds.x: 12.00048828125, expected 8.0 (Δ +4.0, tol ±0.5)"]
    );

    // Rounding is for the eye only: a difference inside the tolerance but
    // outside three decimals is still inside the tolerance, and one outside it
    // is still a delta.
    assert!(deltas_of(&native("\"x\": 8.0", "\"x\": 8.4999")).is_empty());
    assert_eq!(deltas_of(&native("\"x\": 8.0", "\"x\": 8.5001")).len(), 1);
}

/// The **archived output of the first real gate run**, held down verbatim.
///
/// `runs/{ref,native}-294-d4.json` are the actual snapshots the two apps
/// produced at width 294, dark, overflow. Against the v1.0 differ they yielded
/// 26 deltas, eight of which were `border.color` on borders of zero width — a
/// third of the report was noise the contract already said to ignore.
///
/// This asserts the complete ranked list rather than a count, so it fails on a
/// delta that appears, one that disappears, one that moves, and one whose
/// wording changes. It is the regression fixture for the whole of P1.6: the
/// eight zero-width colour deltas are gone, every other finding survives, and
/// the four Δs that used to carry a dozen digits of float noise now read at the
/// contract's three decimals.
#[test]
fn the_archived_gate_run_is_reproduced_delta_for_delta() {
    let reference = include_str!(concat!(env!("CARGO_MANIFEST_DIR"), "/runs/ref-294-d4.json"));
    let native_side = include_str!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/runs/native-294-d4.json"
    ));
    assert_eq!(
        deltas_between(reference, native_side),
        vec![
            "git-row-deleted: anchor present in the native snapshot but not in the reference",
            "git-row-dir: anchor present in the native snapshot but not in the reference",
            "git-row-badge.clipped: not emitted by the native extractor, expected false \
             (field-presence mismatch, not a value delta)",
            "git-row-badge.fg: not emitted by the native extractor, expected #ffb900ff \
             (field-presence mismatch, not a value delta)",
            "git-row-badge.font: not emitted by the native extractor, expected \
             { size: 12.0, weight: 500, family: \"CalSansUI\", line_height: 16.0 } \
             (field-presence mismatch, not a value delta)",
            "git-row-badge.text: not emitted by the native extractor, expected \"uncommitted\" \
             (field-presence mismatch, not a value delta)",
            "git-row-badge.text_width: not emitted by the native extractor, expected 79.33 \
             (field-presence mismatch, not a value delta)",
            "git-row-added.text: \"+12\", expected \"+1\" (exact)",
            "git-row-name.text_width: 424.047, expected 476.49 (Δ -52.443, tol ±1.0)",
            "git-row-name.bounds.w: 39.5, expected 91.52 (Δ -52.02, tol ±0.5)",
            "git-row-added.bounds.x: 252.0, expected 276.84 (Δ -24.84, tol ±0.5)",
            "git-row-badge.bounds.w: 66.0, expected 87.33 (Δ -21.33, tol ±0.5)",
            "git-row-added.bounds.w: 21.0, expected 11.16 (Δ +9.84, tol ±0.5)",
            "git-row-added.text_width: 20.355, expected 11.15 (Δ +9.205, tol ±1.0)",
            "git-row-badge.bounds.h: 16.0, expected 20.0 (Δ -4.0, tol ±0.5)",
            "git-row-badge.bounds.x: 180.0, expected 183.52 (Δ -3.52, tol ±0.5)",
            "git-row-badge.bounds.y: 4.0, expected 2.0 (Δ +2.0, tol ±0.5)",
            "git-row-name.bounds.h: 19.0, expected 18.0 (Δ +1.0, tol ±0.5)",
        ]
    );

    // Eight anchors in that run carry a border of zero width whose two sides
    // report *different* colours — the DOM's inherited `#f5f5f5ff` and
    // `#ffffff0f` against GPUI's `#00000000`. Counted from the fixtures rather
    // than assumed, so a future change that reintroduces them fails here with
    // the reason rather than as an off-by-eight in the list above.
    let reference_snapshot = Snapshot::from_json(reference).expect("archived reference loads");
    let native_snapshot = Snapshot::from_json(native_side).expect("archived native loads");
    let suppressed = reference_snapshot
        .anchors
        .iter()
        .filter(|anchor| {
            let (Some(reference_border), Some(native_border)) = (
                anchor.border,
                native_snapshot.anchor(&anchor.id).and_then(|a| a.border),
            ) else {
                return false;
            };
            (reference_border.w <= 0.0 || native_border.w <= 0.0)
                && reference_border.color != native_border.color
        })
        .count();
    assert_eq!(suppressed, 8);
    let report = diff(&reference_snapshot, &native_snapshot, &Tolerances::V1).expect("same cell");
    assert!(
        !report
            .deltas
            .iter()
            .any(|d| d.field == "border.color" || d.field == "border.w"),
        "the archived run's borders agree once the zero-width colours are ignored: {:?}",
        report.deltas
    );
}

#[test]
fn the_ranking_is_stable_across_runs() {
    let mutated = native("\"x\": 8.0", "\"x\": 12.0");
    let first = deltas_of(&mutated);
    for _ in 0..8 {
        assert_eq!(deltas_of(&mutated), first);
    }
}

#[test]
fn a_different_matrix_cell_is_a_refusal_not_a_delta() {
    let cases: &[(&str, &str, &str, &str)] = &[
        (
            "theme",
            "\"theme\": \"dark\"",
            "\"theme\": \"light\"",
            "state.theme: native light, reference dark",
        ),
        (
            "width",
            "\"width\": 320",
            "\"width\": 640",
            "state.width: native 640, reference 320",
        ),
        (
            "content length",
            "\"content\": \"overflow\"",
            "\"content\": \"short\"",
            "state.content: native short, reference overflow",
        ),
        (
            "flags",
            "\"flags\": [\"selected\", \"hover\"]",
            "\"flags\": [\"hover\"]",
            "state.flags: native [hover], reference [hover, selected]",
        ),
        (
            "surface",
            "\"surface\": \"git-status-row\"",
            "\"surface\": \"sidebar-tree-row\"",
            "surface: native sidebar-tree-row, reference git-status-row",
        ),
    ];
    for (what, from, to, want_line) in cases {
        let expected = Snapshot::from_json(REFERENCE).expect("valid");
        let actual = Snapshot::from_json(&native(from, to)).expect("valid");
        let mismatch = diff(&expected, &actual, &Tolerances::V1)
            .expect_err("differing state must refuse, not produce a delta");
        assert_eq!(mismatch.fields.len(), 1, "case: {what}");
        let rendered = mismatch.to_string();
        assert!(
            rendered.contains(want_line),
            "case {what}: {rendered} does not contain {want_line}"
        );
        assert!(rendered.contains("fake convergence"), "case: {what}");
    }
}

#[test]
fn flag_order_is_not_a_state_difference() {
    // `flags` is a set. Two extractors listing the same flags in a different
    // order are in the same matrix cell, and refusing on that would be a false
    // alarm that trains people to ignore the refusal.
    let reordered = native(
        "\"flags\": [\"selected\", \"hover\"]",
        "\"flags\": [\"hover\", \"selected\"]",
    );
    let expected = Snapshot::from_json(REFERENCE).expect("valid");
    let actual = Snapshot::from_json(&reordered).expect("valid");
    assert!(diff(&expected, &actual, &Tolerances::V1).is_ok());
}

#[test]
fn several_context_fields_differing_are_all_reported() {
    let expected = Snapshot::from_json(REFERENCE).expect("valid");
    let actual = Snapshot::from_json(
        &REFERENCE
            .replace("\"theme\": \"dark\"", "\"theme\": \"light\"")
            .replace("\"width\": 320", "\"width\": 640"),
    )
    .expect("valid");
    let mismatch = diff(&expected, &actual, &Tolerances::V1).expect_err("two differences");
    assert_eq!(mismatch.fields.len(), 2);
    assert_eq!(mismatch.fields[0].field, "state.width");
    assert_eq!(mismatch.fields[1].field, "state.theme");
    let rendered = mismatch.to_string();
    assert!(
        rendered.ends_with(
            "\n  state.width: native 640, reference 320\
             \n  state.theme: native light, reference dark"
        ),
        "{rendered}"
    );
}

#[test]
fn a_schema_mismatch_refuses_even_though_the_loader_pins_the_version() {
    // The loader only accepts `schema: 1`, so two *loaded* snapshots always
    // agree. The check exists for a caller that builds `Snapshot` values
    // directly — an in-process extractor — and is covered from that direction.
    let expected = Snapshot::from_json(REFERENCE).expect("valid");
    let mut actual = expected.clone();
    actual.schema = 2;
    let mismatch = diff(&expected, &actual, &Tolerances::V1).expect_err("schema differs");
    assert_eq!(mismatch.fields.len(), 1);
    assert_eq!(mismatch.fields[0].field, "schema");
    assert_eq!(mismatch.fields[0].expected, "1");
    assert_eq!(mismatch.fields[0].actual, "2");
}

#[test]
fn two_empty_snapshots_compare_clean_but_say_so_loudly() {
    let snapshot = Snapshot::from_json(EMPTY).expect("valid");
    let report = diff(&snapshot, &snapshot, &Tolerances::V1).expect("same cell");
    assert!(report.is_clean());
    // The count is the whole point: "0 deltas over 0 anchors" is a pass on
    // paper and evidence of nothing, and the CLI refuses it by default.
    assert_eq!(report.anchors_compared, 0);
    assert_eq!(report.summary(), "PASS — 0 deltas over 0 anchors compared");
}

#[test]
fn no_shared_anchors_at_all_reports_every_anchor_on_both_sides() {
    let expected = Snapshot::from_json(REFERENCE).expect("valid");
    let actual = Snapshot::from_json(&REFERENCE.replace("git-row-", "other-row-")).expect("valid");
    let report = diff(&expected, &actual, &Tolerances::V1).expect("same cell");
    assert_eq!(report.anchors_compared, 0);
    assert_eq!(report.deltas.len(), 6);
    assert_eq!(
        report.summary(),
        "FAIL — 6 deltas over 0 anchors compared (6 anchor presence)"
    );
}

#[test]
fn tolerances_are_overridable_and_change_the_verdict() {
    let expected = Snapshot::from_json(REFERENCE).expect("valid");
    let actual = Snapshot::from_json(&native("\"x\": 8.0", "\"x\": 10.0")).expect("valid");

    assert_eq!(
        diff(&expected, &actual, &Tolerances::V1)
            .expect("same cell")
            .deltas
            .len(),
        1
    );

    let loosened = Tolerances {
        bounds_px: 2.0,
        ..Tolerances::V1
    };
    assert!(
        diff(&expected, &actual, &loosened)
            .expect("same cell")
            .is_clean(),
        "a loosened tolerance must actually loosen — and this is exactly the change \
         ANCHORS.md §5 requires to land in its own commit with a justification"
    );
}

#[test]
fn malformed_input_is_an_error_with_a_position_not_a_panic() {
    let cases: &[(&str, &str, &str)] = &[
        (
            "colour without alpha",
            "\"bg\": \"#1e2228ff\"",
            "\"bg\": \"#1e2228\"",
        ),
        (
            "colour that is not hex",
            "\"bg\": \"#1e2228ff\"",
            "\"bg\": \"rgb(30,34,40)\"",
        ),
        (
            "colour missing the hash",
            "\"bg\": \"#1e2228ff\"",
            "\"bg\": \"1e2228ff\"",
        ),
        (
            "NaN is not JSON, so it never becomes a number",
            "\"x\": 8.0",
            "\"x\": NaN",
        ),
        (
            "an overflowing literal is not a finite number",
            "\"x\": 8.0",
            "\"x\": 1e999",
        ),
        (
            "a negative overflowing literal likewise",
            "\"x\": 8.0",
            "\"x\": -1e999",
        ),
        (
            "a field the v1 contract does not define",
            "\"visible\": true },\n    { \"id\": \"git-row-name\"",
            "\"visible\": true, \"z\": 3 },\n    { \"id\": \"git-row-name\"",
        ),
        ("truncated document", "\"schema\": 1,", "\"schema\": 1"),
    ];
    for (what, from, to) in cases {
        let json = native(from, to);
        let err = Snapshot::from_json(&json).expect_err(what);
        assert!(!err.to_string().is_empty(), "case {what} rendered empty");
    }
}

#[test]
fn a_malformed_colour_never_silently_becomes_black() {
    // A parser that defaulted on failure would make a garbage colour compare
    // equal to a genuinely black one.
    let err = Snapshot::from_json(&native("\"bg\": \"#1e2228ff\"", "\"bg\": \"#zzzzzzzz\""))
        .expect_err("not hex");
    let rendered = err.to_string();
    assert!(rendered.contains("not a hex digit"), "{rendered}");
    assert!(rendered.contains("anchors[0].bg"), "{rendered}");
}

#[test]
fn a_non_finite_number_is_rejected_at_parse_time() {
    let err = Snapshot::from_json(&native("\"x\": 8.0", "\"x\": 1e999")).expect_err("infinite");
    let rendered = err.to_string();
    assert!(rendered.contains("not a finite number"), "{rendered}");
    assert!(rendered.contains("meaningless"), "{rendered}");
}
