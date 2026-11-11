package mongoadapter

import (
	"testing"

	"geo-assist/internal/catalog/application"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSearchEscapesRegexAndExcludesDeleted(t *testing.T) {
	filter := searchFilter(application.SearchQuery{NamePrefix: " CAFE.* ", Tags: []string{"wifi", "quiet"}})
	if value, ok := filter["deleted_at"]; !ok || value != nil {
		t.Fatal("missing active-only predicate")
	}
	pattern := filter["name_search"].(bson.M)["$regex"]
	if pattern != `^cafe\.\*` {
		t.Fatalf("unsafe regex: %s", pattern)
	}
	if _, ok := filter["tags"].(bson.M)["$all"]; !ok {
		t.Fatal("tags should be AND combined")
	}
}
