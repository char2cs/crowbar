export interface Position {
  line: number
  column: number
  offset: number
}

export interface Range {
  start: Position
  end: Position
}
