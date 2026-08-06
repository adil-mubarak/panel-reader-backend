# Detector calibration fixture schema

`detector-calibration.json` is an array of fixtures. Each fixture has:

- `name`: unique test name and generated image filename.
- `synthetic.width`, `synthetic.height`: generated PNG dimensions in pixels.
- `synthetic.verticalGutters`: optional half-open `{start,end}` x ranges for white gutters.
- `synthetic.horizontalGutters`: optional half-open `{start,end}` y ranges for white gutters.
- `expected`: normalized `[0,1]` boxes with `x`, `y`, `width`, and `height` fields.

The test generates deterministic, non-copyrighted images from this data, invokes the pure-Go detector, and greedily performs one-to-one matching at IoU >= 0.5. It reports precision, recall, and mean IoU over matched boxes per fixture and in aggregate. This is an evaluation/calibration harness only; it does not train or tune a model.
