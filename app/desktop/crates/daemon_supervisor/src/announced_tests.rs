use super::*;

fn step(index: u32) -> Option<MigrationProgress> {
    let detail =
        serde_json::json!({"step": {"number": index, "name": "s"}, "index": index, "total": 3});
    MigrationProgress::parse(&detail)
}

#[test]
fn only_a_change_is_reported_and_the_first_report_always_is() {
    let mut announced = Announced::default();
    let mut heard = 0;
    let mut count = |_: Option<&MigrationProgress>| heard += 1;
    for progress in [None, None, step(1), step(1), step(2), None] {
        announced.report(progress, &mut count);
    }
    assert_eq!(heard, 4, "none, 1, 2, none");
}
