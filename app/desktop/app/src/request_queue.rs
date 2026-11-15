pub(crate) type Requests = async_channel::Sender<crate::shell_request::ShellRequest>;

pub(crate) fn send(
    requests: &Requests,
    request: crate::shell_request::ShellRequest,
) {
    if requests.try_send(request).is_err() {
        tracing::warn!("the shell is closing; a request was dropped");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell_request::ShellRequest;

    #[test]
    fn a_sent_request_reaches_the_queue() {
        let (requests, queue) = async_channel::unbounded();
        send(&requests, ShellRequest::CloseSettings);
        assert_eq!(
            queue.try_recv(),
            Ok(ShellRequest::CloseSettings),
            "delivered"
        );
    }

    #[test]
    fn a_request_to_a_closed_shell_is_dropped_without_failing() {
        let (requests, queue) = async_channel::unbounded();
        drop(queue);
        send(&requests, ShellRequest::CloseSettings);
        assert!(requests.is_closed(), "nobody is listening");
    }
}
