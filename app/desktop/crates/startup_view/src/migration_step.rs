/// The migration step the daemon reports: its name and where it sits in the plan.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MigrationStep {
    name: String,
    index: u32,
    total: u32,
}

impl MigrationStep {
    /// Step `index` of `total` (both from 1) called `name`.
    pub fn new(name: impl Into<String>, index: u32, total: u32) -> Self {
        Self {
            name: name.into(),
            index,
            total,
        }
    }

    /// The step's name.
    pub fn name(&self) -> &str {
        &self.name
    }

    /// Which step this is, from 1.
    pub fn index(&self) -> u32 {
        self.index
    }

    /// How many steps the migration has.
    pub fn total(&self) -> u32 {
        self.total
    }

    /// `Step 2 of 5: name`, the line the old toast showed.
    pub fn label(&self) -> String {
        format!("Step {} of {}: {}", self.index, self.total, self.name)
    }

    /// `index / total` in `0.0..=1.0`, or `None` when the plan has no steps.
    #[expect(
        clippy::cast_precision_loss,
        reason = "step counts are tiny, f32 holds them exactly"
    )]
    pub fn fraction(&self) -> Option<f32> {
        if self.total == 0 {
            return None;
        }
        Some((self.index as f32 / self.total as f32).clamp(0.0, 1.0))
    }
}

#[cfg(test)]
#[path = "migration_step_tests.rs"]
mod tests;
