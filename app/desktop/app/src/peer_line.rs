use browser_agent::{Error, PeerStream};
use transport::TextStream;

pub(crate) struct PeerLine {
    stream: TextStream,
}

impl PeerLine {
    pub(crate) fn new(stream: TextStream) -> Self {
        Self { stream }
    }
}

impl PeerStream for PeerLine {
    async fn recv(&mut self) -> Option<String> {
        self.stream.recv().await
    }

    async fn send(
        &mut self,
        text: String,
    ) -> Result<(), Error> {
        self.stream
            .send(text)
            .await
            .map_err(|error| Error::Stream(Box::new(error)))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn the_peer_reads_and_writes_the_lines_of_its_stream_and_a_dead_one_is_an_error() {
        let (near, mut far) = TextStream::loopback().await.unwrap();
        let mut line = PeerLine::new(near);
        far.send("hello".into()).await.unwrap();
        assert_eq!(line.recv().await.as_deref(), Some("hello"));
        line.send("back".into()).await.unwrap();
        assert_eq!(far.recv().await.as_deref(), Some("back"));
        drop(far);
        assert_eq!(line.recv().await, None);
        assert!(matches!(
            line.send("late".into()).await,
            Err(Error::Stream(_))
        ));
    }
}
