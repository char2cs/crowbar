const BAND_MARGIN: f32 = 32.0;

#[must_use]
pub(crate) fn pull(
    pointer_y: f32,
    row_top: f32,
    grab_y: f32,
) -> f32 {
    let delta = pointer_y - grab_y - row_top;
    (delta.abs() / BAND_MARGIN).clamp(0.0, 1.0)
}

#[must_use]
pub(crate) fn ghost_y(
    pointer_y: f32,
    row_top: f32,
    grab_y: f32,
) -> f32 {
    let delta = pointer_y - grab_y - row_top;
    row_top + delta * pull(pointer_y, row_top, grab_y)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_ghost_is_pinned_to_the_row_near_it_and_free_far_from_it() {
        assert_eq!(ghost_y(26.0, 5.0, 21.0), 5.0, "level with the row");
        let eased = ghost_y(26.0 + 16.0, 5.0, 21.0);
        assert!(eased > 5.0 && eased < 21.0, "halfway eased: {eased}");
        assert_eq!(
            ghost_y(26.0 + 200.0, 5.0, 21.0),
            205.0,
            "free beyond the margin"
        );
    }
}
