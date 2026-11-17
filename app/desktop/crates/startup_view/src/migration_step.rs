/// The migration step the daemon reports: its name and where it sits in the plan.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MigrationStep {
    name: String,
    index: u32,
    total: u32,
}

impl MigrationStep {
    /// Step `index` of `total` (both from 1) called `name`.
    pub fn new(
        name: impl Into<String>,
        index: u32,
        total: u32,
    ) -> Self {
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
    pub fn fraction(&self) -> Option<f32> {
        if self.total == 0 {
            return None;
        }
        let index = f32::from(u16::try_from(self.index).unwrap_or(u16::MAX));
        let total = f32::from(u16::try_from(self.total).unwrap_or(u16::MAX));
        Some((index / total).clamp(0.0, 1.0))
    }
}

#[cfg(test)]
mod tests {
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
}
