/// Works.
pub fn work(f: impl Fn(bool) -> u32) -> u32 {
    f(true)
}
#[cfg(test)]
mod tests {}
