use std::time::Duration;

use tokio::time::Instant;

use crate::verdict::Verdict;

pub(crate) struct RestartBudget {
    window: Duration,
    max: usize,
    restarts: Vec<Instant>,
}

impl RestartBudget {
    pub(crate) fn new(max: usize, window: Duration) -> Self {
        Self {
            window,
            max,
            restarts: Vec::new(),
        }
    }

    pub(crate) fn on_death(&mut self, now: Instant) -> Verdict {
        let window = self.window;
        self.restarts
            .retain(|at| now.saturating_duration_since(*at) < window);
        if self.restarts.len() >= self.max {
            return Verdict::GiveUp {
                deaths: self.restarts.len() + 1,
            };
        }
        self.restarts.push(now);
        Verdict::Respawn {
            attempt: self.restarts.len(),
        }
    }

    pub(crate) fn reset(&mut self) {
        self.restarts.clear();
    }
}

#[cfg(test)]
#[path = "restart_budget_tests.rs"]
mod tests;
