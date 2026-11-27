/// Works.
pub fn work(
    (a, b): (u32, u32),
    (c, d): (u32, u32),
    e: u32,
) -> u32 {
    a + b + c + d + e
}

#[cfg(test)]
mod tests {}
