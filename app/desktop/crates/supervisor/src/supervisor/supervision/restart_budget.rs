use std::time::Duration;

use tokio::time::Instant;

use crate::supervisor::supervision::verdict::Verdict;

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
            return self.exhausted();
        }
        self.restarts.push(now);
        Verdict::Respawn {
            attempt: self.restarts.len(),
        }
    }

    fn exhausted(&self) -> Verdict {
        Verdict::GiveUp {
            deaths: self.restarts.len() + 1,
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
        let respawn = |attempt| Verdict::Respawn { attempt };
        let give_up = |deaths| Verdict::GiveUp { deaths };
        let cases = [
            (
                "budget spent inside the window",
                3,
                600,
                vec![0, 1, 2, 3],
                vec![respawn(1), respawn(2), respawn(3), give_up(4)],
            ),
            (
                "old restarts age out of the window",
                1,
                10,
                vec![0, 5, 11],
                vec![respawn(1), give_up(2), respawn(1)],
            ),
        ];
        for (name, max, window, deaths, expected) in cases {
            let start = Instant::now();
            let mut budget = RestartBudget::new(max, Duration::from_secs(window));
            let verdicts: Vec<Verdict> = deaths
                .into_iter()
                .map(|secs| budget.on_death(start + Duration::from_secs(secs)))
                .collect();
            assert_eq!(verdicts, expected, "{name}");
        }
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
