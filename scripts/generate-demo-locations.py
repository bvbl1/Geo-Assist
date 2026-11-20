"""Prepare fixed synthetic locations near named Astana streets, never real POIs.

Usage: python scripts/generate-demo-locations.py path/to/roads.json
Input: OSM ways in elements with id, tags and geometry [{lat, lon}, ...].
Accepts Overpass JSON or map API XML converted into the same structure.
The application embeds the result and never downloads streets at runtime.
"""
import json
import math
import random
import sys
from pathlib import Path

data = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
south, west, north, east = 51.065, 71.32, 51.235, 71.56
xscale, yscale = 111320 * math.cos(math.radians(51.15)), 111320
candidates = []
for way in sorted(data["elements"], key=lambda w: w["id"]):
    tags = way.get("tags", {})
    if not tags.get("name:ru", tags.get("name", "")):
        continue
    if tags.get("bridge", "no") != "no" or tags.get("tunnel", "no") != "no":
        continue
    if tags.get("highway") not in {"residential", "tertiary", "secondary", "unclassified"}:
        continue
    geometry = way.get("geometry", [])
    for a, b in zip(geometry, geometry[1:]):
        dx, dy = (b["lon"] - a["lon"]) * xscale, (b["lat"] - a["lat"]) * yscale
        length = math.hypot(dx, dy)
        if length < 65:
            continue
        # Avoid junction endpoints; move 10 m to the side of the street centreline.
        for fraction in ([0.25, 0.5, 0.75] if length > 250 else [0.5]):
            lon = a["lon"] + fraction * (b["lon"] - a["lon"])
            lat = a["lat"] + fraction * (b["lat"] - a["lat"])
            offset = 10 if way["id"] % 2 else -10
            point = [round(lon - offset * dy / length / xscale, 7),
                     round(lat + offset * dx / length / yscale, 7)]
            if not (west < point[0] < east and south < point[1] < north):
                continue
            candidates.append({"street": tags.get("name:ru", tags.get("name", "")),
                               "osm_way_id": way["id"], "coordinates": point,
                               "street_anchor": [round(lon, 7), round(lat, 7)]})
assert len(candidates) >= 100, "Not enough named street segments"

def distance2(a, b):
    return ((a[0]-b[0])*xscale)**2 + ((a[1]-b[1])*yscale)**2

# Farthest-point selection spreads the sample across the street network, not a grid.
selected = [min(candidates, key=lambda p: distance2(p["coordinates"], [71.4304, 51.1282]))]
nearest = [distance2(p["coordinates"], selected[0]["coordinates"]) for p in candidates]
while len(selected) < 100:
    idx = max(range(len(candidates)), key=lambda i: nearest[i])
    selected.append(candidates[idx])
    nearest = [min(d, distance2(p["coordinates"], selected[-1]["coordinates"]))
               for d, p in zip(nearest, candidates)]
random.Random(20261114).shuffle(selected)
output = {"version": 2, "source": "© OpenStreetMap contributors",
          "source_url": "https://www.openstreetmap.org/copyright",
          "license": "ODbL 1.0", "osm_timestamp": data.get("osm3s", {}).get("timestamp_osm_base"),
          "retrieved_date": data.get("retrieved_date"), "geometry_source": data.get("source", "Overpass API"),
          "sample_centers_lat_lon": data.get("sample_centers", []),
          "description": "Synthetic demo points offset 10 m from named street segments; not real venues or addresses.",
          "locations": selected}
target = Path(__file__).resolve().parents[1]/"internal/platform/mongostore/astana-demo-locations.json"
target.write_text(json.dumps(output, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")
print(f"Saved {len(selected)} locations on {len(set(p['street'] for p in selected))} streets to {target}")
