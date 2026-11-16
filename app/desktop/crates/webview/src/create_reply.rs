use std::sync::mpsc;

use crate::error::Result;
use crate::web_view_id::WebViewId;

pub(crate) struct CreateReply {
    pub(crate) reply: mpsc::Sender<Result<()>>,
    pub(crate) id: WebViewId,
}

impl CreateReply {
    pub(crate) fn send(
        self,
        created: Result<()>,
    ) -> Option<WebViewId> {
        let Err(mpsc::SendError(Ok(()))) = self.reply.send(created) else {
            return None;
        };
        tracing::warn!("web view created for a caller that stopped waiting");
        Some(self.id)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_creation_nobody_waits_for_gives_its_view_back_to_be_destroyed() {
        let (reply, waiting) = mpsc::channel();
        let answered = CreateReply {
            reply: reply.clone(),
            id: WebViewId(7),
        }
        .send(Ok(()));
        assert_eq!((answered, waiting.try_recv().is_ok()), (None, true));
        drop(waiting);
        let abandoned = CreateReply {
            reply,
            id: WebViewId(8),
        }
        .send(Ok(()));
        assert_eq!(abandoned, Some(WebViewId(8)));
    }
}
