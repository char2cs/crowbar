use super::*;

#[test]
fn the_crash_report_is_the_last_run_of_the_log() {
    let log = "=== daemon spawned (epoch 1)\n[err] old run\n=== daemon terminated (code=Some(2))\n\
               === daemon spawned (epoch 2)\n[err] panic: boom\n[err] goroutine 1\n\
               === daemon terminated (code=Some(2))\n";
    assert_eq!(
        last_run_tail(log, 10),
        "=== daemon spawned (epoch 2)\n[err] panic: boom\n[err] goroutine 1\n=== daemon terminated (code=Some(2))"
    );
    assert_eq!(
        last_run_tail(log, 2),
        "[err] goroutine 1\n=== daemon terminated (code=Some(2))"
    );
    assert_eq!(last_run_tail("no marker\nat all", 5), "no marker\nat all");
}

#[test]
fn the_summary_names_the_deaths_counted_and_the_window() {
    assert_eq!(
        crash_summary(4, Duration::from_secs(600)),
        "The backend exited 4 times within 10 minutes."
    );
}

#[test]
fn an_unreadable_log_is_said_so_in_the_report() {
    let report = crash_report("It died.", Path::new("/definitely/not/here.log"));
    assert!(report.contains("It died."));
    assert!(report.contains("could not read /definitely/not/here.log"));
}
