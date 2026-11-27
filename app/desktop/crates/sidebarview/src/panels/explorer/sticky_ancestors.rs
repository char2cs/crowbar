pub(crate) fn top_row(
    scrolled: f32,
    (row_height, count): (f32, usize),
) -> usize {
    let bottoms = std::iter::successors(Some(row_height), |bottom| Some(bottom + row_height));
    let past = bottoms.take(count).position(|bottom| bottom > scrolled);
    past.unwrap_or(count.saturating_sub(1))
}

pub(crate) fn sticky_ancestors(
    depths: &[u8],
    top: usize,
) -> Vec<usize> {
    let (Some(&depth), Some(above)) = (depths.get(top), depths.get(..top)) else {
        return Vec::new();
    };
    let mut wanted = depth;
    let innermost_first: Vec<usize> = above
        .iter()
        .enumerate()
        .rev()
        .filter(|&(_, &candidate)| {
            let encloses = candidate < wanted;
            wanted = wanted.min(candidate);
            encloses
        })
        .map(|(at, _)| at)
        .collect();
    innermost_first.into_iter().rev().collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    const DEPTHS: [u8; 6] = [0, 1, 2, 2, 1, 0];

    #[test]
    fn the_row_at_the_top_pins_each_folder_it_sits_in_outermost_first() {
        let cases: [(usize, &[usize]); 5] =
            [(0, &[]), (1, &[0]), (3, &[0, 1]), (4, &[0]), (5, &[])];
        for (top, want) in cases {
            assert_eq!(sticky_ancestors(&DEPTHS, top), want, "top {top}");
        }
    }

    #[test]
    fn the_top_row_is_the_first_whose_bottom_is_past_the_scroll() {
        let cases = [
            (0.0, 0),
            (-4.0, 0),
            (25.9, 0),
            (26.0, 1),
            (60.0, 2),
            (900.0, 5),
        ];
        for (scrolled, want) in cases {
            assert_eq!(top_row(scrolled, (26.0, DEPTHS.len())), want, "{scrolled}");
        }
    }
}
