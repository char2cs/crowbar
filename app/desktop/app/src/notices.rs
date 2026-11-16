use sidebar::DockSide;
use ui::ToastPlacement;

pub(crate) fn floating_at(side: DockSide) -> ToastPlacement {
    ToastPlacement::Floating(side.toast_side())
}

#[cfg(test)]
mod tests {
    use ui::ToastSide;

    use super::*;

    #[test]
    fn floating_notices_hug_the_dock_edge() {
        assert_eq!(
            floating_at(DockSide::Left),
            ToastPlacement::Floating(ToastSide::Left)
        );
        assert_eq!(
            floating_at(DockSide::Right),
            ToastPlacement::Floating(ToastSide::Right)
        );
    }
}
