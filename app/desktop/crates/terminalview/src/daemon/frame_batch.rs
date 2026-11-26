//! How much daemon output the view applies in one UI update.

use async_channel::Receiver;
use terminal::DaemonFrame;

const BATCH_BYTES: usize = 256 * 1024;

pub(crate) fn take_batch(
    first: DaemonFrame,
    frames: &Receiver<DaemonFrame>,
) -> Vec<DaemonFrame> {
    let mut bytes = 0;
    let mut pending = Some(first);
    let drawn = std::iter::from_fn(|| {
        let frame = pending.take()?;
        bytes += weight(&frame);
        pending = (bytes < BATCH_BYTES)
            .then(|| frames.try_recv().ok())
            .flatten();
        Some(frame)
    });
    drawn.fold(Vec::new(), |mut batch, frame| {
        push(&mut batch, frame);
        batch
    })
}

fn weight(frame: &DaemonFrame) -> usize {
    match frame {
        DaemonFrame::Output(bytes) | DaemonFrame::Snapshot(bytes) => bytes.len(),
        DaemonFrame::Exit(_) => 0,
    }
}

fn push(
    batch: &mut Vec<DaemonFrame>,
    frame: DaemonFrame,
) {
    if let (Some(DaemonFrame::Output(tail)), DaemonFrame::Output(more)) = (batch.last_mut(), &frame)
    {
        tail.extend_from_slice(more);
        return;
    }
    batch.push(frame);
}

#[cfg(test)]
mod tests {
    use async_channel::unbounded;

    use super::*;

    fn output(len: usize) -> DaemonFrame {
        DaemonFrame::Output(vec![b'x'; len])
    }

    #[test]
    fn output_merges_and_other_frames_stay_apart() {
        for after in [DaemonFrame::Exit(0), DaemonFrame::Snapshot(vec![1])] {
            let (sender, frames) = unbounded();
            sender.try_send(output(2)).unwrap();
            sender.try_send(after.clone()).unwrap();
            let expected = vec![output(3), after];
            assert_eq!(take_batch(output(1), &frames), expected);
        }
    }

    #[test]
    fn a_batch_stops_at_its_byte_budget_and_leaves_the_rest_queued() {
        let (sender, frames) = unbounded();
        for _ in 0..4 {
            sender.try_send(output(BATCH_BYTES / 2)).unwrap();
        }
        let batch = take_batch(output(BATCH_BYTES / 2), &frames);
        assert_eq!(batch, [output(BATCH_BYTES)]);
        assert_eq!(frames.len(), 3);
    }
}
