use crate::migration_progress::MigrationProgress;

#[derive(Default)]
pub(crate) struct Announced {
    any: bool,
    last: Option<MigrationProgress>,
}

impl Announced {
    pub(crate) fn report(
        &mut self,
        progress: Option<MigrationProgress>,
        on_migrating: &mut impl FnMut(Option<&MigrationProgress>),
    ) {
        if self.any && self.last == progress {
            return;
        }
        on_migrating(progress.as_ref());
        self.any = true;
        self.last = progress;
    }
}

#[cfg(test)]
#[path = "announced_tests.rs"]
mod tests;
