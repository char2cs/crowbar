/// Works.
pub fn work<F: Fn(u32) -> u32>(a: u32, f: F) -> u32 {
    f(a)
}

#[cfg(test)]
mod tests {}
