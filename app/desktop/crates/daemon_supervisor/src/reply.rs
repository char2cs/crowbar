use tokio::sync::oneshot;

pub(crate) fn reply<T>(sender: oneshot::Sender<T>, value: T) {
    if sender.send(value).is_err() {
        tracing::debug!("a supervisor command was abandoned before its answer");
    }
}

#[cfg(test)]
#[path = "reply_tests.rs"]
mod tests;
