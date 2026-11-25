use crate::shell::window::readout::frame_stats::FrameStats;
use crate::shell::window::readout::readout_net::ReadoutNet;
use crate::shell::window::readout::readout_snapshot::ReadoutSnapshot;

const SEGMENT_GAP: &str = "  ";

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub(crate) struct ReadoutLines {
    pub(crate) build: String,
    pub(crate) net: String,
    pub(crate) stats: String,
    pub(crate) warns: bool,
}

fn frame_text(frame: FrameStats) -> String {
    let fps = match frame.fps {
        0 => "--".to_owned(),
        fps => fps.to_string(),
    };
    let gap = match frame.max_gap_ms {
        0 => "--ms".to_owned(),
        ms => format!("{ms}ms"),
    };
    let drops = match frame.drops {
        0 => String::new(),
        drops => format!("/{drops}"),
    };
    format!("{fps}fps {gap}{drops}")
}

pub(crate) fn lines_of(snapshot: &ReadoutSnapshot) -> ReadoutLines {
    let frame = frame_text(snapshot.frame);
    let (pty, ag, ws) = snapshot.counts;
    let reconnects = match snapshot.reconnects {
        0 => None,
        count => Some(format!("rc{count}")),
    };
    let net = ["net ".to_owned() + snapshot.net.label()]
        .into_iter()
        .chain(reconnects)
        .collect::<Vec<_>>()
        .join(SEGMENT_GAP);
    let stats = match snapshot.net {
        ReadoutNet::Down => frame,
        _ => format!("{frame}{SEGMENT_GAP}pty{pty} ag{ag} ws{ws}"),
    };
    ReadoutLines {
        build: snapshot.build.clone(),
        net,
        stats,
        warns: snapshot.net.warns(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn snapshot(
        net: ReadoutNet,
        reconnects: u32,
        frame: FrameStats,
    ) -> ReadoutSnapshot {
        ReadoutSnapshot {
            build: "dev 0.1.0".into(),
            net,
            reconnects,
            frame,
            counts: (2, 1, 3),
        }
    }

    #[test]
    fn the_lines_name_the_net_the_frame_and_the_counts() {
        let busy = FrameStats {
            fps: 60,
            max_gap_ms: 18,
            drops: 2,
        };
        let cases = [
            (
                snapshot(ReadoutNet::Ok, 0, busy),
                ("net ok", "60fps 18ms/2  pty2 ag1 ws3", false),
            ),
            (
                snapshot(ReadoutNet::Reconnecting, 3, FrameStats::default()),
                ("net reconnecting  rc3", "--fps --ms  pty2 ag1 ws3", true),
            ),
            (
                snapshot(ReadoutNet::Down, 0, busy),
                ("net DOWN", "60fps 18ms/2", true),
            ),
            (
                snapshot(ReadoutNet::Idle, 0, FrameStats::default()),
                ("net --", "--fps --ms  pty2 ag1 ws3", false),
            ),
        ];
        for (snapshot, (net, stats, warns)) in cases {
            let lines = lines_of(&snapshot);
            assert_eq!(
                (lines.net.as_str(), lines.stats.as_str(), lines.warns),
                (net, stats, warns)
            );
            assert_eq!(lines.build, "dev 0.1.0");
        }
    }
}
