use super::*;

fn every_outage() -> [Outage; 4] {
    [
        Outage::Unresponsive,
        Outage::Recovering {
            attempt: 2,
            reason: "exit status 1".to_owned(),
        },
        Outage::GaveUp {
            report: "crowbar 1.0".to_owned(),
        },
        Outage::Stopped,
    ]
}

#[test]
fn only_a_supervisor_that_gave_up_or_stopped_offers_retry() {
    let retryable: Vec<bool> = every_outage().iter().map(Outage::is_retryable).collect();
    assert_eq!(retryable, [false, false, true, true]);
    let restarting: Vec<bool> = every_outage().iter().map(Outage::is_restarting).collect();
    assert_eq!(restarting, [true, true, false, false]);
}

#[test]
fn every_outage_explains_itself() {
    for outage in every_outage() {
        assert!(!outage.headline().is_empty(), "{outage:?}");
        assert!(!outage.explanation().is_empty(), "{outage:?}");
    }
}

#[test]
fn detail_is_the_reason_or_the_report() {
    let outages = every_outage();
    let details: Vec<Option<&str>> = outages.iter().map(Outage::detail).collect();
    assert_eq!(
        details,
        [None, Some("exit status 1"), Some("crowbar 1.0"), None]
    );
}

#[test]
fn recovering_names_the_attempt() {
    let outage = Outage::Recovering {
        attempt: 3,
        reason: String::new(),
    };
    assert!(outage.explanation().contains("attempt 3"));
}
