use crate::probe::Probe;

pub(crate) struct FailureTracker {
    threshold: u32,
    consecutive: u32,
}

impl FailureTracker {
    pub(crate) fn new(threshold: u32) -> Self {
        Self {
            threshold,
            consecutive: 0,
        }
    }

    pub(crate) fn reset(&mut self) {
        self.consecutive = 0;
    }

    pub(crate) fn observe(&mut self, outcome: Probe) -> bool {
        if outcome != Probe::Unserved {
            self.reset();
            return false;
        }
        self.consecutive = self.consecutive.saturating_add(1);
        self.consecutive == self.threshold
    }
}

#[cfg(test)]
#[path = "failure_tracker_tests.rs"]
mod tests;
