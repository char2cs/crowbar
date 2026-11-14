use serde_json::json;

use super::*;

const FIXTURE: &str =
    include_str!("../../../../../api/internal/core/bootgate/testdata/health-migrating.json");

fn detail(body: &Value) -> &Value {
    &body["data"]["detail"]
}

#[test]
fn the_daemons_own_migrating_answer_reads_as_step_two_of_five() {
    let body: Value = serde_json::from_str(FIXTURE).unwrap();
    let progress = MigrationProgress::parse(detail(&body)).unwrap();
    assert_eq!(progress.step(), "workspace-paths-table");
    assert_eq!((progress.index(), progress.total()), (1, 5));
}

#[test]
fn a_step_without_a_name_is_called_by_its_number() {
    let detail = json!({"step": {"number": 4}, "index": 3, "total": 5});
    assert_eq!(MigrationProgress::parse(&detail).unwrap().step(), "step 4");
    let blank = json!({"step": {"number": 4, "name": ""}, "index": 3, "total": 5});
    assert_eq!(MigrationProgress::parse(&blank).unwrap().step(), "step 4");
}

#[test]
fn no_step_yet_is_no_progress() {
    let cases = [
        Value::Null,
        json!({}),
        json!({"phase": "pre-open"}),
        json!({"step": "x", "index": 1, "total": 2}),
        json!({"step": {"name": "a"}, "index": "1", "total": 2}),
        json!({"step": {"name": "a"}, "index": 1}),
        json!({"step": {"name": "a"}, "index": -1, "total": 2}),
    ];
    for case in cases {
        assert_eq!(MigrationProgress::parse(&case), None, "{case}");
    }
}
