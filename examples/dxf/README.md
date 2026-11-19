# Checking a traced DXF before you cut

`check_dxf.py` prints the numbers a laser or CNC program cares about: entity
count, closed against open polylines, vertex count, layers, extents in the
file's units, and the DXF version.

```
pip install ezdxf
python3 check_dxf.py panel.dxf
```

```
entities     69  {'LWPOLYLINE': 69}
closed       69
open          0
vertices   7355
layers           ['rekha_000000']
size             100.1 x 100.1 millimetres
version          AC1015
```

Three things to read before pressing start:

- **Size.** The extents are the shape, not the canvas. A 1600 px image written
  at the default 96 dpi is 423 mm wide; set the dpi so the image is the width
  you planned (1600 px / 100 mm × 25.4 = 406.4).
- **Closed equals total.** One open polyline is one piece that stays in the sheet.
- **Curve type.** Arcs cut smoother and make a smaller file; a polyline-only
  file is safest for older controllers.

The file carries no kerf. Beam width is set on the machine.

DXF export is in the Rekha studio at https://app.rekhasvg.com/ on the Pro plan.
Video: https://www.youtube.com/@rekhasvg
