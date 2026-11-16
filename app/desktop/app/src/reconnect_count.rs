use crate::readout_net::ReadoutNet;

/// The reconnect count after the link moved from `before` to `now`: a drop from `Ok` into
/// trouble counts once, so the first connection at startup does not.
pub(crate) fn recount(
    count: u32,
    (before, now): (ReadoutNet, ReadoutNet),
) -> u32 {
    let dropped =
        before == ReadoutNet::Ok && matches!(now, ReadoutNet::Reconnecting | ReadoutNet::Down);
    count + u32::from(dropped)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_drop_from_ok_into_trouble_counts() {
        const CASES: [(ReadoutNet, ReadoutNet, u32); 5] = [
            (ReadoutNet::Ok, ReadoutNet::Down, 1),
            (ReadoutNet::Ok, ReadoutNet::Reconnecting, 1),
            (ReadoutNet::Down, ReadoutNet::Ok, 0),
            (ReadoutNet::Idle, ReadoutNet::Down, 0),
            (ReadoutNet::Ok, ReadoutNet::Ok, 0),
        ];
        for (before, now, added) in CASES {
            assert_eq!(recount(0, (before, now)), added, "{before:?} -> {now:?}");
        }
    }
}
