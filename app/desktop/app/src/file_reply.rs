use shell_data::FileDone;

pub(crate) type Reply<T> = async_channel::Receiver<Result<T, String>>;

fn hand_over<T>(
    sender: &async_channel::Sender<Result<T, String>>,
    result: Result<T, String>,
) {
    if sender.try_send(result).is_err() {
        tracing::debug!("the shell stopped waiting for a file action");
    }
}

pub(crate) fn channel<T: Send + 'static>() -> (FileDone<T>, Reply<T>) {
    let (sender, receiver) = async_channel::bounded(1);
    let done: FileDone<T> = Box::new(move |result| hand_over(&sender, result));
    (done, receiver)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_answer_of_the_action_reaches_the_receiver_once() {
        let (done, reply) = channel::<u8>();
        done(Ok(7));
        assert_eq!(reply.try_recv(), Ok(Ok(7)));
        let (done, reply) = channel::<u8>();
        done(Err("no".into()));
        assert_eq!(reply.try_recv(), Ok(Err("no".to_owned())));
    }
}
