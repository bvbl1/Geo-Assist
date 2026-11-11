package domain

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	shared "geo-assist/internal/shared/domain"

	"golang.org/x/text/unicode/norm"
)

type Category string

const (
	Cafe       Category = "cafe"
	Park       Category = "park"
	Library    Category = "library"
	Coworking  Category = "coworking"
	Attraction Category = "attraction"
)

func ValidCategory(category Category) bool {
	switch category {
	case Cafe, Park, Library, Coworking, Attraction:
		return true
	default:
		return false
	}
}

type Coordinates struct {
	Longitude float64
	Latitude  float64
}

type Place struct {
	ID          shared.ID
	Name        string
	Description string
	Category    Category
	Tags        []string
	Address     string
	Location    Coordinates
	CreatedBy   shared.ID
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

type PlaceHit struct {
	Place     Place
	DistanceM *float64
}

// FieldError keeps validation independent of HTTP and persistence.
type FieldError struct{ Field, Reason string }

func (e *FieldError) Error() string { return e.Field + ": " + e.Reason }

func NormalizeName(name string) string {
	return strings.ToLower(norm.NFC.String(strings.TrimSpace(name)))
}

func ValidateCoordinates(c Coordinates) error {
	if math.IsNaN(c.Longitude) || math.IsInf(c.Longitude, 0) || c.Longitude < -180 || c.Longitude > 180 {
		return &FieldError{"location.longitude", "out_of_range"}
	}
	if math.IsNaN(c.Latitude) || math.IsInf(c.Latitude, 0) || c.Latitude < -90 || c.Latitude > 90 {
		return &FieldError{"location.latitude", "out_of_range"}
	}
	return nil
}

var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func NormalizeTags(tags []string) ([]string, error) {
	if len(tags) > 20 {
		return nil, &FieldError{"tags", "too_many"}
	}
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, original := range tags {
		tag := strings.ToLower(strings.TrimSpace(original))
		if !tagPattern.MatchString(tag) {
			return nil, &FieldError{"tags", "invalid_slug"}
		}
		if seen[tag] {
			return nil, &FieldError{"tags", "duplicate"}
		}
		seen[tag] = true
		result = append(result, tag)
	}
	sort.Strings(result)
	return result, nil
}

// NormalizeAndValidate is also used after applying a partial update.
func (p *Place) NormalizeAndValidate() error {
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	p.Address = strings.TrimSpace(p.Address)
	for _, field := range []struct {
		name, value string
		max         int
	}{
		{"name", p.Name, 120}, {"description", p.Description, 2000}, {"address", p.Address, 300},
	} {
		if !utf8.ValidString(field.value) || utf8.RuneCountInString(field.value) < 1 || utf8.RuneCountInString(field.value) > field.max {
			return &FieldError{field.name, "invalid_length"}
		}
	}
	if !ValidCategory(p.Category) {
		return &FieldError{"category", "invalid_category"}
	}
	var err error
	p.Tags, err = NormalizeTags(p.Tags)
	if err != nil {
		return err
	}
	return ValidateCoordinates(p.Location)
}
