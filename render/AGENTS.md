# Render package guide

## Responsibility

`render` converts layout connectivity and labels into terminal text. It chooses
Unicode glyphs but does not place nodes or route edges.

## Core data

- `Frame` stores encoded bytes and document-space bounds
- `Encoder` owns reusable grid, label, symbol, endpoint, continuation, and
  line buffers
- package functions provide convenient allocating entry points
- `Encoder` methods serve interactive hot paths

`layout.Rasterize` produces connectivity and ownership. Rendering then places
labels, arrows, borders, and stroke styles. Keep that order so labels and
endpoint symbols resolve against final ownership.

Multiline labels use `layout.AppendLabelLines` and
`Layout.LabelLinePoint`. Explicit nodes wrap to their inner width and clip
outside their visible bounds. Automatic nodes already fit their contents.

## Invariants

- glyph choice must not affect routing or hit testing
- spaces used for visual styles must not remove raster ownership
- arrow direction follows the side of the port it enters
- endpoint arrows replace the final line cell
- joined edge connectivity must produce T and crossing glyphs consistently
- front complete nodes occlude lower unrelated geometry
- encoding must preserve grapheme display width

## Performance

Use one retained `Encoder` for repeated frames. Grow and clear scratch instead
of reallocating it. Preview rasterization should share normal glyph resolution
so committed and transient edges cannot drift visually.

## Areas for improvement

Current dashed strokes use Unicode double-dash cells. Their horizontal and
vertical rhythm looks uneven in typical terminal cells.

Phase-aware dashing should retain solid connectivity and ownership, then:

- render horizontal runs as two line cells followed by two spaces
- render vertical runs as one line cell followed by one space
- keep corners, junctions, arrows, and ports solid
- fit the phase to each complete segment so endpoints remain visible

Custom shapes will need a renderable composition format, a label owner, and
horizontal and vertical scaling rules. Keep reusable shape definitions outside
the committed raster format.

## Verification

Keep exact examples for glyphs, arrows, shared borders, occlusion, alignment,
and wrapping. Use headless-terminal for font and cell-rhythm decisions.
Benchmark retained `Encoder` paths with `-benchmem`.
