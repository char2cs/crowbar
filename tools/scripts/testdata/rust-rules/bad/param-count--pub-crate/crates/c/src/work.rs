/// Works.
pub(crate) fn work<T: Into<String>>(
    a: u32,
    b: u32,
    c: u32,
    d: u32,
    e: T,
) -> u32 {
    a + b + c + d + e.into().len() as u32
}

#[cfg(test)]
mod tests {}
