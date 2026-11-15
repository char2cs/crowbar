use crate::error::Error;

pub(crate) async fn blocking<T, F>(work: F) -> Result<T, Error>
where
    T: Send + 'static,
    F: FnOnce() -> Result<T, Error> + Send + 'static,
{
    tokio::task::spawn_blocking(work)
        .await
        .map_err(|e| Error::BlockingTask(e.to_string()))?
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn the_work_runs_off_the_async_workers_and_its_result_comes_back() {
        assert_eq!(blocking(|| Ok(7)).await.unwrap(), 7);
        assert!(matches!(
            blocking::<(), _>(|| Err(Error::TooLarge)).await,
            Err(Error::TooLarge)
        ));
    }

    #[tokio::test]
    async fn a_panicking_task_is_an_error_not_a_crash() {
        let result = blocking::<(), _>(|| std::panic::panic_any("boom")).await;
        assert!(matches!(result, Err(Error::BlockingTask(_))), "{result:?}");
    }
}
