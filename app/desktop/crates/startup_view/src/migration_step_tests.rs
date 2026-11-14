use super::*;

#[test]
fn fraction_is_index_over_total_and_clamped() {
    type Case = (u32, u32, Option<f32>);
    const CASES: &[Case] = &[
        (1, 4, Some(0.25)),
        (4, 4, Some(1.0)),
        (9, 4, Some(1.0)),
        (0, 4, Some(0.0)),
        (1, 0, None),
    ];
    for &(index, total, expected) in CASES {
        let step = MigrationStep::new("rebuild", index, total);
        assert_eq!(step.fraction(), expected, "{index} of {total}");
    }
}

#[test]
fn label_reads_like_the_old_toast() {
    let step = MigrationStep::new("Rebuild index", 2, 5);
    assert_eq!(step.label(), "Step 2 of 5: Rebuild index");
}
