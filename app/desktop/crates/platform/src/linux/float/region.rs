use x11rb::protocol::xproto::Rectangle;

use crate::linux::float::card::Card;

pub(crate) fn rectangles(cards: &[Card]) -> Vec<Rectangle> {
    cards.iter().flat_map(card_rows).collect()
}

fn card_rows(card: &Card) -> Vec<Rectangle> {
    let radius = card.radius.min(card.width / 2).min(card.height / 2);
    let middle = Rectangle {
        x: card.x,
        y: offset(card.y, radius),
        width: card.width,
        height: card.height - 2 * radius,
    };
    let bottom = offset(card.y, card.height - 1);
    let curve = (0..radius).flat_map(|row| curve_rows(card, (radius, row), bottom));
    std::iter::once(middle)
        .filter(|rect| rect.height > 0)
        .chain(curve)
        .collect()
}

fn curve_rows(
    card: &Card,
    (radius, row): (u16, u16),
    bottom: i16,
) -> [Rectangle; 2] {
    let inset = inset(radius, row);
    let width = card.width - 2 * inset;
    let x = offset(card.x, inset);
    let strip = |y| Rectangle {
        x,
        y,
        width,
        height: 1,
    };
    [
        strip(offset(card.y, row)),
        strip(bottom.saturating_sub_unsigned(row)),
    ]
}

fn inset(
    radius: u16,
    row: u16,
) -> u16 {
    let whole = f32::from(radius);
    let rise = whole - f32::from(row) - 0.5;
    (0..radius)
        .find(|&column| {
            let run = whole - f32::from(column) - 0.5;
            run * run + rise * rise <= whole * whole
        })
        .unwrap_or(radius)
}

fn offset(
    start: i16,
    by: u16,
) -> i16 {
    start.saturating_add_unsigned(by)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn card(radius: u16) -> Card {
        Card {
            x: 10,
            y: 20,
            width: 40,
            height: 30,
            radius,
        }
    }

    #[test]
    fn a_rounded_card_narrows_its_first_and_last_rows_and_covers_every_row_once() {
        let rows = rectangles(&[card(4)]);
        let covered: u16 = rows.iter().map(|rect| rect.height).sum();
        assert_eq!(covered, 30, "every row once");
        let top = rows
            .iter()
            .find(|rect| rect.y == 20)
            .copied()
            .unwrap_or_default();
        let bottom = rows
            .iter()
            .find(|rect| rect.y == 49)
            .copied()
            .unwrap_or_default();
        assert!(top.x > 10 && top.width < 40, "{top:?}");
        assert_eq!((bottom.x, bottom.width), (top.x, top.width), "mirrored");
        let middle = rows
            .iter()
            .find(|rect| rect.y == 24)
            .copied()
            .unwrap_or_default();
        assert_eq!((middle.x, middle.width, middle.height), (10, 40, 22));
    }

    #[test]
    fn a_card_of_any_radius_covers_each_of_its_rows_once() {
        const CASES: [(u16, usize); 3] = [(0, 1), (4, 9), (100, 30)];
        for (radius, count) in CASES {
            let rows = rectangles(&[card(radius)]);
            let covered: u16 = rows.iter().map(|rect| rect.height).sum();
            assert_eq!((covered, rows.len()), (30, count), "radius {radius}");
        }
    }
}
