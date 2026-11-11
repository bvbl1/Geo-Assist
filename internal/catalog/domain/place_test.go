package domain

import (
	"math"
	"reflect"
	"testing"
)

func validPlace() Place {
	return Place{Name: " Example Coffee ", Description: " Quiet place ", Address: " Astana demo address ",
		Category: Cafe, Tags: []string{"WiFi", "quiet"}, Location: Coordinates{Longitude: 71.4304, Latitude: 51.1282}}
}

func TestNormalizeAndValidate(t *testing.T) {
	p := validPlace()
	if err := p.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Example Coffee" || p.Address != "Astana demo address" {
		t.Fatalf("strings not trimmed: %+v", p)
	}
	if !reflect.DeepEqual(p.Tags, []string{"quiet", "wifi"}) {
		t.Fatalf("unexpected tags: %v", p.Tags)
	}
	if NormalizeName("  CAF\u0045\u0301  ") != "café" {
		t.Fatal("name normalization must use NFC and lowercase")
	}
}

func TestCoordinatesRejectInvalidNumbers(t *testing.T) {
	for name, coordinates := range map[string]Coordinates{
		"longitude": {Longitude: 181}, "latitude": {Latitude: -91},
		"nan": {Longitude: math.NaN()}, "infinity": {Latitude: math.Inf(1)},
	} {
		t.Run(name, func(t *testing.T) {
			if ValidateCoordinates(coordinates) == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTagsAndCategoriesAreValidated(t *testing.T) {
	for _, tags := range [][]string{{"wifi", "WiFi"}, {"with space"}, {"$where"}} {
		if _, err := NormalizeTags(tags); err == nil {
			t.Fatalf("accepted invalid tags %v", tags)
		}
	}
	p := validPlace()
	p.Category = "unknown"
	if p.NormalizeAndValidate() == nil {
		t.Fatal("accepted unknown category")
	}
	p = validPlace()
	p.Tags = nil
	if err := p.NormalizeAndValidate(); err != nil || p.Tags == nil {
		t.Fatal("empty tags must normalize to non-nil array")
	}
}
