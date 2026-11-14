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
#[path = "blocking_tests.rs"]
mod tests;
