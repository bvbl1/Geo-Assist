package mongostore

import (
	"math"
	"testing"
)

func TestDemoLocationsSpreadAlongStreets(t *testing.T) {
	seen := map[[2]float64]bool{}
	minLat, maxLat, minLon, maxLon := 90., -90., 180., -180.
	for _, p := range demoStreetLocations {
		lon, lat := p.Coordinates[0], p.Coordinates[1]
		if p.Street == "" || lon < 71.32 || lon > 71.56 || lat < 51.065 || lat > 51.235 {
			t.Fatalf("invalid Astana point: %+v", p)
		}
		if seen[p.Coordinates] {
			t.Fatal("duplicate coordinates")
		}
		seen[p.Coordinates] = true
		distance := math.Hypot((lon-p.Anchor[0])*111320*math.Cos(lat*math.Pi/180), (lat-p.Anchor[1])*111320)
		if distance < 8 || distance > 12 {
			t.Fatalf("point %.1fm from its street anchor", distance)
		}
		minLat = math.Min(minLat, lat)
		maxLat = math.Max(maxLat, lat)
		minLon = math.Min(minLon, lon)
		maxLon = math.Max(maxLon, lon)
	}
	if maxLat-minLat < .1 || maxLon-minLon < .12 {
		t.Fatal("demo points do not span the city")
	}
}
