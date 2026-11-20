package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

const Algorithm = "item_cf_jaccard"

type Edge struct{ User, Place string }
type Source struct {
	Places []string
	Edges  []Edge
}
type Neighbor struct {
	PlaceID         string  `bson:"place_id" json:"place_id"`
	Similarity      float64 `bson:"similarity" json:"similarity"`
	CofavoriteCount int64   `bson:"cofavorite_count" json:"cofavorite_count"`
}
type Item struct {
	PlaceID       string     `bson:"place_id"`
	FavoriteCount int64      `bson:"favorite_count"`
	Neighbors     []Neighbor `bson:"neighbors"`
}
type Model struct {
	Items       map[string]Item
	Fingerprint string
	PairCount   int
}

func (s Source) Fingerprint() string {
	places := append([]string{}, s.Places...)
	sort.Strings(places)
	edges := append([]Edge{}, s.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].User == edges[j].User {
			return edges[i].Place < edges[j].Place
		}
		return edges[i].User < edges[j].User
	})
	h := sha256.New()
	fmt.Fprintln(h, Algorithm, "1")
	for _, p := range places {
		fmt.Fprintln(h, "p", p)
	}
	for _, e := range edges {
		fmt.Fprintln(h, "e", e.User, e.Place)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Build uses an inverted user -> favorites index. No positive neighbors are truncated.
func Build(ctx context.Context, s Source) (*Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(s.Places) > 10000 || len(s.Edges) > 200000 {
		return nil, fmt.Errorf("source size limit exceeded")
	}
	m := &Model{Items: map[string]Item{}, Fingerprint: s.Fingerprint()}
	for _, p := range s.Places {
		m.Items[p] = Item{PlaceID: p, Neighbors: []Neighbor{}}
	}
	users := map[string]map[string]bool{}
	for _, e := range s.Edges {
		if _, ok := m.Items[e.Place]; !ok {
			continue
		}
		if users[e.User] == nil {
			users[e.User] = map[string]bool{}
		}
		users[e.User][e.Place] = true
	}
	pairs := map[[2]string]int64{}
	for _, set := range users {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(set))
		for id := range set {
			ids = append(ids, id)
			item := m.Items[id]
			item.FavoriteCount++
			m.Items[id] = item
		}
		sort.Strings(ids)
		for i, a := range ids {
			for _, b := range ids[i+1:] {
				pairs[[2]string{a, b}]++
				if len(pairs) > 1000000 {
					return nil, fmt.Errorf("pair limit exceeded")
				}
			}
		}
	}
	for pair, n := range pairs {
		a, b := m.Items[pair[0]], m.Items[pair[1]]
		sim := float64(n) / float64(a.FavoriteCount+b.FavoriteCount-n)
		a.Neighbors = append(a.Neighbors, Neighbor{b.PlaceID, sim, n})
		b.Neighbors = append(b.Neighbors, Neighbor{a.PlaceID, sim, n})
		m.Items[a.PlaceID] = a
		m.Items[b.PlaceID] = b
	}
	for id, item := range m.Items {
		sort.Slice(item.Neighbors, func(i, j int) bool { return item.Neighbors[i].PlaceID < item.Neighbors[j].PlaceID })
		m.Items[id] = item
	}
	m.PairCount = len(pairs)
	return m, nil
}

func (m *Model) Scores(favorites []string) map[string]float64 {
	scores := map[string]float64{}
	seen := map[string]bool{}
	// Stable addition order prevents floating-point ties changing with cursor order.
	ordered := append([]string{}, favorites...)
	sort.Strings(ordered)
	for _, id := range ordered {
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, n := range m.Items[id].Neighbors {
			scores[n.PlaceID] += n.Similarity
		}
	}
	for id := range seen {
		delete(scores, id)
	}
	return scores
}
