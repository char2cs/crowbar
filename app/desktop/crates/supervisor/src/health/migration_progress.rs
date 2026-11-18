use serde_json::Value;

/// Where a migrating daemon is: the step it works on and how many there are, as `/v0/health`
/// reports them. Enough for "Step 2 of 5: name" and a determinate progress bar.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MigrationProgress {
    step: String,
    index: u32,
    total: u32,
}

impl MigrationProgress {
    pub(crate) fn parse(detail: &Value) -> Option<Self> {
        let number = |value: Option<&Value>| {
            value
                .and_then(Value::as_u64)
                .and_then(|n| u32::try_from(n).ok())
        };
        let step = detail.get("step").filter(|step| step.is_object())?;
        let index = number(detail.get("index"))?;
        let total = number(detail.get("total"))?;
        let name = match step.get("name").and_then(Value::as_str) {
            Some(name) if !name.is_empty() => name.to_owned(),
            _ => format!("step {}", number(step.get("number")).unwrap_or(index)),
        };
        Some(Self {
            step: name,
            index,
            total,
        })
    }

    /// The step's name.
    #[must_use]
    pub fn step(&self) -> &str {
        &self.step
    }

    /// Which step this is, from 1.
    #[must_use]
    pub fn index(&self) -> u32 {
        self.index
    }

    /// How many steps the migration has.
    #[must_use]
    pub fn total(&self) -> u32 {
        self.total
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    const FIXTURE: &str =
        include_str!("../../../../../../api/internal/core/bootgate/testdata/health-migrating.json");

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
}
