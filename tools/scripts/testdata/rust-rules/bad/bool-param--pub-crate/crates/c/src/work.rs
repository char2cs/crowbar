pub(crate) fn work(
    a: u32,
    flag: bool,
) -> u32 {
    a + u32::from(flag)
}

#[cfg(test)]
mod tests {}
