#!/usr/bin/env python3
"""check_dxf.py FILE.dxf — the numbers a laser cutter cares about. Needs: pip install ezdxf"""
import sys
from collections import Counter
import ezdxf
doc = ezdxf.readfile(sys.argv[1]); msp = doc.modelspace()
kinds = Counter(e.dxftype() for e in msp)
polys = [e for e in msp if e.dxftype() == "LWPOLYLINE"]
closed = sum(1 for e in polys if e.closed)
mx, mn = doc.header["$EXTMAX"], doc.header["$EXTMIN"]
units = {0: "unitless", 1: "inches", 4: "millimetres", 5: "centimetres"}.get(doc.header.get("$INSUNITS", 0), "?")
print(f"entities  {sum(kinds.values()):5d}  {dict(kinds)}")
print(f"closed    {closed:5d}")
print(f"open      {len(polys) - closed:5d}")
print(f"vertices  {sum(len(e) for e in polys):5d}")
print(f"layers           {[l.dxf.name for l in doc.layers if l.dxf.name not in ('0', 'Defpoints')]}")
print(f"size             {mx[0] - mn[0]:.1f} x {mx[1] - mn[1]:.1f} {units}")
print(f"version          {doc.dxfversion}")
