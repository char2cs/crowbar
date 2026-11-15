use std::time::Duration;

use tokio::time::Instant;

use crate::verdict::Verdict;

pub(crate) struct RestartBudget {
    window: Duration,
    max: usize,
    restarts: Vec<Instant>,
}

impl RestartBudget {
    pub(crate) fn new(
        max: usize,
        window: Duration,
    ) -> Self {
        Self {
            window,
            max,
            restarts: Vec::new(),
        }
    }

    pub(crate) fn on_death(
        &mut self,
        now: Instant,
    ) -> Verdict {
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
mod tests {
    use super::*;

    #[test]
    fn restarts_are_budgeted_inside_a_sliding_window() {
        let start = Instant::now();
        let mut budget = RestartBudget::new(3, Duration::from_secs(600));
        assert_eq!(budget.on_death(start), Verdict::Respawn { attempt: 1 });
        assert_eq!(
            budget.on_death(start + Duration::from_secs(1)),
            Verdict::Respawn { attempt: 2 }
        );
        assert_eq!(
            budget.on_death(start + Duration::from_secs(2)),
            Verdict::Respawn { attempt: 3 }
        );
        assert_eq!(
            budget.on_death(start + Duration::from_secs(3)),
            Verdict::GiveUp { deaths: 4 }
        );
    }

    #[test]
    fn old_restarts_age_out_of_the_window() {
        let start = Instant::now();
        let mut budget = RestartBudget::new(1, Duration::from_secs(10));
        assert_eq!(budget.on_death(start), Verdict::Respawn { attempt: 1 });
        assert_eq!(
            budget.on_death(start + Duration::from_secs(5)),
            Verdict::GiveUp { deaths: 2 }
        );
        assert_eq!(
            budget.on_death(start + Duration::from_secs(11)),
            Verdict::Respawn { attempt: 1 }
        );
    }

    #[test]
    fn a_manual_restart_starts_a_fresh_budget() {
        let start = Instant::now();
        let mut budget = RestartBudget::new(1, Duration::from_secs(600));
        budget.on_death(start);
        budget.reset();
        assert_eq!(budget.on_death(start), Verdict::Respawn { attempt: 1 });
    }
}
