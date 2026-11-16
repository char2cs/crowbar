/// W.
public struct W {
  func f(_ xs: [Int]) -> [Int] {
    var doubled = [Int]()
    for x in xs { doubled.append(x + x) }
    return doubled
  }
}
