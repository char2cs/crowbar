use model::{PaneSlot, Rect};
use protocol::PaneId;

use crate::direction::Direction;

pub(crate) fn focus_neighbour(
    slots: &[PaneSlot],
    from: &PaneId,
    direction: Direction,
) -> Option<PaneId> {
    let origin = slots.iter().find(|slot| &slot.pane == from)?.rect;
    let nearest = |slot: &&PaneSlot| nearness(origin, slot, from, direction);
    slots
        .iter()
        .filter_map(|slot| nearest(&slot).map(|gap| (gap, slot)))
        .min_by(|a, b| a.0.total_cmp(&b.0))
        .map(|(_, slot)| slot.pane.clone())
}

fn nearness(
    origin: Rect,
    slot: &PaneSlot,
    from: &PaneId,
    direction: Direction,
) -> Option<f32> {
    if &slot.pane == from || !overlaps_across(origin, slot.rect, direction) {
        return None;
    }
    gap(origin, slot.rect, direction)
}

fn overlaps_across(
    a: Rect,
    b: Rect,
    direction: Direction,
) -> bool {
    match direction {
        Direction::Left | Direction::Right => a.y < b.y + b.height && b.y < a.y + a.height,
        Direction::Up | Direction::Down => a.x < b.x + b.width && b.x < a.x + a.width,
    }
}

fn gap(
    from: Rect,
    to: Rect,
    direction: Direction,
) -> Option<f32> {
    let distance = match direction {
        Direction::Left => from.x - (to.x + to.width),
        Direction::Right => to.x - (from.x + from.width),
        Direction::Up => from.y - (to.y + to.height),
        Direction::Down => to.y - (from.y + from.height),
    };
    (distance >= 0.0).then_some(distance)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn slot(
        id: &str,
        rect: Rect,
    ) -> PaneSlot {
        PaneSlot {
            pane: PaneId::new(id).unwrap(),
            rect,
        }
    }

    fn rect(
        x: f32,
        y: f32,
        size: (f32, f32),
    ) -> Rect {
        Rect {
            x,
            y,
            width: size.0,
            height: size.1,
        }
    }

    fn id(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    fn grid() -> Vec<PaneSlot> {
        vec![
            slot("a", rect(0.0, 0.0, (100.0, 50.0))),
            slot("b", rect(0.0, 54.0, (100.0, 46.0))),
            slot("c", rect(104.0, 0.0, (96.0, 100.0))),
        ]
    }

    #[test]
    fn focus_moves_to_the_adjacent_pane_on_that_side() {
        let slots = grid();
        let cases = [
            ("a", Direction::Right, Some("c")),
            ("a", Direction::Down, Some("b")),
            ("b", Direction::Up, Some("a")),
            ("c", Direction::Left, Some("a")),
            ("a", Direction::Left, None),
            ("c", Direction::Down, None),
        ];
        for (from, direction, expected) in cases {
            let found = focus_neighbour(&slots, &id(from), direction);
            assert_eq!(found, expected.map(id), "{from} {direction:?}");
        }
    }

    #[test]
    fn an_unknown_pane_has_no_neighbour() {
        assert_eq!(focus_neighbour(&grid(), &id("zz"), Direction::Left), None);
    }
}
