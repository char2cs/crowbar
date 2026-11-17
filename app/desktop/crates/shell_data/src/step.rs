//! What a stream session woke up for.

use tokio::sync::mpsc;

use crate::stream_control::StreamControl;

pub(crate) enum Step {
    Frame(Option<String>),
    Control(Option<StreamControl>),
}

pub(crate) async fn next_step(
    incoming: &mut mpsc::Receiver<String>,
    controls: &mut mpsc::UnboundedReceiver<StreamControl>,
) -> Step {
    tokio::select! {
        text = incoming.recv() => Step::Frame(text),
        control = controls.recv() => Step::Control(control),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn a_frame_and_a_control_each_wake_the_session_as_their_own_step() {
        let (frames, mut incoming) = mpsc::channel(1);
        let (controls, mut inbox) = mpsc::unbounded_channel();
        frames.send("text".to_owned()).await.unwrap();
        assert!(matches!(
            next_step(&mut incoming, &mut inbox).await,
            Step::Frame(Some(text)) if text == "text"
        ));
        controls.send(StreamControl::Resubscribe).unwrap();
        assert!(matches!(
            next_step(&mut incoming, &mut inbox).await,
            Step::Control(Some(_))
        ));
    }
}
