pub(crate) async fn proc_stat(pid: u32) -> Option<String> {
    tokio::fs::read_to_string(format!("/proc/{pid}/stat"))
        .await
        .map_err(|e| tracing::trace!("/proc/{pid}/stat: {e}"))
        .ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn a_process_has_a_stat_line_only_while_it_exists() {
        let own = proc_stat(std::process::id()).await;
        assert!(own.is_none_or(|stat| stat.contains(')')));
        assert_eq!(proc_stat(i32::MAX as u32).await, None);
    }
}
