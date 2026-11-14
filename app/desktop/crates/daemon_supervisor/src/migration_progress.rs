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
#[path = "migration_progress_tests.rs"]
mod tests;
