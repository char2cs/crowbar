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
