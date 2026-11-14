use super::*;

const MINUTE: Duration = Duration::from_secs(60);

#[test]
fn a_cycle_far_longer_than_nominal_means_the_process_was_suspended() {
    let (interval, probe, margin) = (
        Duration::from_secs(10),
        Duration::from_secs(5),
        Duration::from_secs(15),
    );
    for secs in [10, 15, 30] {
        assert!(
            !cycle_was_suspended(Duration::from_secs(secs), interval, probe, margin),
            "{secs}"
        );
    }
    for secs in [31, 3600] {
        assert!(
            cycle_was_suspended(Duration::from_secs(secs), interval, probe, margin),
            "{secs}"
        );
    }
}

#[test]
fn only_a_running_unresponsive_idle_daemon_is_killed() {
    assert!(should_kill_wedged(&[]));
    for reason in [Exoneration::Suspended, Exoneration::Answering, Exoneration::Busy] {
        assert!(!should_kill_wedged(&[reason]), "{reason:?}");
    }
    assert!(!should_kill_wedged(&[Exoneration::Suspended, Exoneration::Answering]));
}

#[test]
fn progress_is_cpu_spent_across_the_unresponsive_stretch() {
    let at = Duration::from_secs;
    assert!(made_progress(Some(at(10)), Some(at(20)), MINUTE));
    assert!(made_progress(
        Some(at(10)),
        Some(at(10) + Duration::from_millis(600)),
        MINUTE
    ));
    assert!(!made_progress(
        Some(at(10)),
        Some(at(10) + Duration::from_millis(50)),
        MINUTE
    ));
    assert!(!made_progress(None, Some(at(20)), MINUTE));
    assert!(!made_progress(Some(at(10)), None, MINUTE));
}

#[test]
fn proc_stat_cpu_reads_utime_plus_stime() {
    let stat = "4242 (crowbar api) S 1 4242 4242 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 9 0";
    assert_eq!(parse_proc_stat_cpu(stat, 100), Some(Duration::from_secs(3)));
    assert_eq!(parse_proc_stat_cpu("4242 (x) S 1", 100), None);
    assert_eq!(parse_proc_stat_cpu(stat, 0), None);
}

#[test]
fn ps_cpu_time_reads_both_clock_formats() {
    assert_eq!(
        parse_ps_cpu_time("00:01:05\n"),
        Some(Duration::from_secs(65))
    );
    assert_eq!(
        parse_ps_cpu_time("2-01:00:00"),
        Some(Duration::from_hours(49))
    );
    assert_eq!(
        parse_ps_cpu_time(" 1:02.50"),
        Some(Duration::from_millis(62_500))
    );
    assert_eq!(parse_ps_cpu_time(""), None);
    assert_eq!(parse_ps_cpu_time("garbage"), None);
}

#[test]
fn a_stopped_process_state_is_t() {
    assert!(proc_state_is_stopped("4242 (crowbar api) T 1 4242"));
    assert!(!proc_state_is_stopped("4242 (crowbar api) S 1 4242"));
    assert!(!proc_state_is_stopped("4242 (T) S 1"));
}
