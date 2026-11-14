use super::*;

#[tokio::test]
async fn a_running_process_has_cpu_time_and_is_not_suspended() {
    let pid = std::process::id();
    assert!(cpu_time(pid).await.is_some());
    assert!(!is_suspended(pid).await);
}

#[tokio::test]
async fn a_process_that_does_not_exist_reads_as_unknown_and_running() {
    let gone = i32::MAX as u32;
    assert_eq!(cpu_time(gone).await, None);
    assert!(!is_suspended(gone).await);
}
