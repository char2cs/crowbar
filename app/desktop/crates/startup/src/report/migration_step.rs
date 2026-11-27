#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MigrationStep {
    name: String,
    index: u32,
    total: u32,
}

impl MigrationStep {
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

    pub fn label(&self) -> String {
        format!("Step {} of {}: {}", self.index, self.total, self.name)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn label_reads_like_the_upgrade_toast() {
        let step = MigrationStep::new("Rebuild index", 2, 5);
        assert_eq!(step.label(), "Step 2 of 5: Rebuild index");
    }
}
