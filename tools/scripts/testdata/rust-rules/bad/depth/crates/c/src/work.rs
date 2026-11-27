/// Works.
pub fn work(a: bool, b: bool) -> u32 {
    if a {
        if b {
            if a {
                return 3;
            }
        }
    }
    0
}
#[cfg(test)]
mod tests {}
