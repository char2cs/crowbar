use super::*;

#[test]
fn the_defaults_give_a_wedged_daemon_every_chance_before_it_is_killed() {
    let t = Timings::default();
    assert!(
        t.grace_probe_timeout > t.probe_timeout,
        "the last probe is the most generous"
    );
    assert!(t.respawn_cap >= t.respawn_base);
    assert!(t.restart_max >= 1);
    assert!(t.failure_threshold >= 2, "one slow probe is never a wedge");
    assert!(t.sigterm_grace >= Duration::from_secs(1));
}
