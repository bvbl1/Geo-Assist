package domain

import (
	"context"
	"math"
	"math/rand"
	"testing"
)

func toy() Source {
	return Source{Places: []string{"A", "B", "C", "D"}, Edges: []Edge{{"u1", "A"}, {"u1", "B"}, {"u2", "A"}, {"u2", "C"}, {"u3", "A"}, {"u3", "C"}, {"u4", "B"}, {"u4", "D"}}}
}
func TestJaccardToy(t *testing.T) {
	m, err := Build(context.Background(), toy())
	if err != nil {
		t.Fatal(err)
	}
	scores := m.Scores([]string{"A", "B"})
	if math.Abs(scores["C"]-2.0/3) > 1e-12 || scores["D"] != .5 || scores["A"] != 0 || scores["B"] != 0 {
		t.Fatalf("scores: %v", scores)
	}
	if m.Scores([]string{"A", "C"})["B"] != .25 {
		t.Fatal("u2 should receive B at .25")
	}
}
func TestExactAgainstPairwiseOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for sample := 0; sample < 30; sample++ {
		s := Source{Places: []string{"A", "B", "C", "D", "E", "F"}}
		sets := map[string]map[int]bool{}
		for _, p := range s.Places {
			sets[p] = map[int]bool{}
			for u := 0; u < 20; u++ {
				if rng.Intn(4) == 0 {
					s.Edges = append(s.Edges, Edge{string(rune('a' + u)), p})
					sets[p][u] = true
				}
			}
		}
		m, err := Build(context.Background(), s)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range s.Places {
			got := m.Scores([]string{a})
			for _, b := range s.Places {
				if a == b {
					continue
				}
				n := 0
				for u := range sets[a] {
					if sets[b][u] {
						n++
					}
				}
				want := 0.0
				if n > 0 {
					want = float64(n) / float64(len(sets[a])+len(sets[b])-n)
				}
				if math.Abs(got[b]-want) > 1e-12 {
					t.Fatalf("%s/%s: %v != %v", a, b, got[b], want)
				}
			}
		}
	}
}
func TestFingerprintAndCancellation(t *testing.T) {
	s := toy()
	hash := s.Fingerprint()
	s.Edges[0], s.Edges[1] = s.Edges[1], s.Edges[0]
	if s.Fingerprint() != hash {
		t.Fatal("fingerprint depends on order")
	}
	s.Edges = s.Edges[1:]
	if s.Fingerprint() == hash {
		t.Fatal("removed favorite not detected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, s); err == nil {
		t.Fatal("cancelled build succeeded")
	}
}
func TestDedupAndNoSignal(t *testing.T) {
	s := toy()
	s.Edges = append(s.Edges, s.Edges[0], Edge{"u1", "deleted"})
	m, err := Build(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if m.Items["A"].FavoriteCount != 3 {
		t.Fatal("duplicate favorite counted")
	}
	if len(m.Scores(nil)) != 0 {
		t.Fatal("cold user should have no CF scores")
	}
	if len(m.Scores([]string{"A", "A"})) != len(m.Scores([]string{"A"})) {
		t.Fatal("duplicate input")
	}
}
