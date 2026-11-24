package rules

import (
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/duanap/gandengyan/server/internal/card"
)

func TestComparison(t *testing.T) {
	s := func(r int) Shape { return Shape{Single, r, 1} }
	p := func(r int) Shape { return Shape{Pair, r, 2} }
	b := func(r, l int) Shape { return Shape{Bomb, r, l} }
	st := func(r, l int) Shape { return Shape{Straight, r, l} }
	pr := func(r, l int) Shape { return Shape{PairRun, r, l} }
	for _, tc := range []struct {
		id         string
		prev, next Shape
		want       bool
	}{
		{"R-01", s(5), s(6), true}, {"R-02", s(5), s(7), false}, {"R-02-A", s(5), s(14), false},
		{"R-03", s(5), s(15), true}, {"R-04", s(15), s(14), false}, {"R-05", p(5), p(6), true},
		{"R-05-2", p(5), p(15), true}, {"R-06", p(5), p(7), false}, {"R-06-A", p(5), p(14), false}, {"R-07", p(15), p(14), false},
		{"R-08", st(3, 3), st(4, 3), true}, {"R-09", st(3, 3), st(5, 3), false}, {"R-09-size", st(3, 3), st(3, 4), false},
		{"R-10", st(3, 4), st(4, 4), true}, {"R-11", st(3, 4), st(5, 4), false}, {"R-11-size", st(3, 4), st(4, 3), false},
		{"R-12", st(11, 3), st(12, 3), true}, {"R-14", st(12, 3), st(11, 3), false},
		{"R-16", pr(3, 4), pr(4, 4), true}, {"R-17", pr(3, 4), pr(5, 4), false}, {"R-17-size", pr(3, 4), pr(3, 6), false},
		{"R-18", pr(3, 6), pr(4, 6), true}, {"R-26", b(3, 3), b(3, 3), false},
		{"R-27-5", b(3, 3), b(5, 3), true}, {"R-27-A", b(3, 3), b(14, 3), true}, {"R-27-2", b(3, 3), b(15, 3), true},
		{"R-28", b(15, 3), b(3, 4), true}, {"R-29", b(15, 4), b(3, 5), true}, {"R-30", b(15, 4), b(14, 4), false},
		{"R-31", b(15, 5), b(3, 6), true}, {"R-32", b(15, 6), b(14, 6), false}, {"R-33", pr(13, 4), b(3, 3), true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			if CanBeat(tc.next, tc.prev) != tc.want {
				t.Fatalf("%v over %v", tc.next, tc.prev)
			}
		})
	}
	for _, template := range allTemplates {
		a := template.Shape
		if CanBeat(a, a) {
			t.Fatalf("self beats: %v", a)
		}
		for _, other := range allTemplates {
			b := other.Shape
			if CanBeat(a, b) && CanBeat(b, a) {
				t.Fatalf("mutual beat: %v %v", a, b)
			}
		}
	}
	for i := 3; i <= 6; i++ {
		for j := 3; j <= 15; j++ {
			a := b(j, i)
			for k := 3; k <= 6; k++ {
				for l := 3; l <= 15; l++ {
					next := b(l, k)
					if !CanBeat(next, a) {
						continue
					}
					for m := 3; m <= 6; m++ {
						for n := 3; n <= 15; n++ {
							last := b(n, m)
							if CanBeat(last, next) && !CanBeat(last, a) {
								t.Fatal("bomb order not transitive")
							}
						}
					}
				}
			}
		}
	}
}

func TestResolveAndDeclare(t *testing.T) {
	long := []card.ID{}
	for i := 0; i < 48; i += 4 {
		long = append(long, card.ID(i))
	}
	for _, tc := range []struct {
		id    string
		cards []card.ID
		count int
	}{
		{"R-13-KA2", []card.ID{40, 44, 48}, 0}, {"R-13-A23", []card.ID{44, 48, 0}, 0}, {"R-13-234", []card.ID{48, 0, 4}, 0},
		{"R-15", long, 1}, {"R-19", []card.ID{44, 45, 48, 49}, 0}, {"R-19-long", []card.ID{40, 41, 44, 45, 48, 49}, 0},
		{"R-20-single", []card.ID{52}, 0}, {"R-20-double", []card.ID{52, 53}, 0}, {"R-21-small", []card.ID{8, 52}, 1},
		{"R-21-big", []card.ID{8, 53}, 1}, {"R-22", []card.ID{4, 8, 53}, 2}, {"R-24/R-37", []card.ID{0, 52, 53}, 2},
		{"R-25-natural", []card.ID{0, 1, 2}, 1}, {"R-25-wild", []card.ID{0, 1, 53}, 1},
		{"R-34", []card.ID{0, 1, 2, 4}, 0}, {"R-34-airplane", []card.ID{0, 1, 2, 4, 5, 6}, 0},
	} {
		t.Run(tc.id, func(t *testing.T) {
			if p := ResolvePatterns(tc.cards); len(p) != tc.count {
				t.Fatalf("%v: %v", tc.cards, p)
			}
		})
	}
	ids := []card.ID{4, 8, 53}
	ctx := Context{Hand: ids}
	if _, err := ValidatePlay(ctx, Request{CardIDs: ids}); err != ChoiceRequired {
		t.Fatal(err)
	}
	ctx.Previous = &Shape{Straight, 3, 3}
	p, err := ValidatePlay(ctx, Request{CardIDs: ids})
	if err != nil || p.Rank != 4 || !slices.Equal(p.Wildcards, []Wildcard{{53, 6}}) {
		t.Fatalf("R-23: %v %v", p, err)
	}
	for _, bad := range [][]card.ID{{4, 4}, {54}, {-1}, {5}, {}} {
		if _, err := Preview(ctx, bad); err != InvalidCards {
			t.Fatalf("R-35 %v: %v", bad, err)
		}
	}
	for _, bad := range []Declaration{{Shape: Shape{Straight, 5, 3}}, {Shape: p.Shape, Wildcards: &[]Wildcard{{53, 5}}}} {
		if _, err := ValidatePlay(ctx, Request{ids, &bad}); err != InvalidPattern {
			t.Fatalf("R-36: %v", err)
		}
	}
	if ResolvePatterns([]card.ID{0, 1, 2})[0].Shape != ResolvePatterns([]card.ID{0, 1, 53})[0].Shape ||
		!slices.ContainsFunc(ResolvePatterns([]card.ID{0, 52, 53}), func(p Pattern) bool { return p.Shape == (Shape{Bomb, 3, 3}) }) {
		t.Fatal("R-25 equivalence")
	}
}

func TestSpecials(t *testing.T) {
	ids := []card.ID{0, 4, 8}
	for i, rest := range [][]card.ID{{48}, {48, 49}, {52}, {52, 53}, {48, 53}, {48, 49, 50}, {48, 49, 50, 51}, {48, 49, 52}, {48, 52, 53}, {48, 49, 50, 51, 52, 53}} {
		t.Run(fmt.Sprintf("S-%02d", i+1), func(t *testing.T) {
			hand := append(slices.Clone(ids), rest...)
			before := slices.Clone(hand)
			_, err := Preview(Context{Hand: hand}, ids)
			if (i < 5 && err != InvalidRemaining) || (i >= 5 && err != nil) || !slices.Equal(hand, before) {
				t.Fatalf("err=%v hand=%v", err, hand)
			}
		})
	}
	for _, rest := range [][]card.ID{{48}, {48, 49}, {48, 53}} {
		if _, err := Preview(Context{Hand: append(slices.Clone(ids), rest...)}, rest); err != nil {
			t.Fatalf("S-11/12 %v", err)
		}
		if _, err := Preview(Context{Hand: rest}, rest); err != InvalidFinal {
			t.Fatalf("S-13 %v", err)
		}
	}
	for _, hand := range [][]card.ID{{8, 52}, {48, 49, 50}, {48, 49, 50, 51}, {48, 49, 50, 51, 53}, {48, 49, 50, 51, 52, 53}, {0, 52, 53}} {
		if !CanFinish(Context{Hand: hand}) {
			t.Fatalf("S-14/15/16 %v", hand)
		}
	}
}

func actionKey(p Pattern) string {
	var ranks []int
	for _, id := range p.CardIDs {
		ranks = append(ranks, id.Rank())
	}
	sort.Ints(ranks)
	return fmt.Sprint(p.Shape, ranks)
}

func TestEnumerateAgainstSubsets(t *testing.T) {
	rng := rand.New(rand.NewSource(9319))
	for sample := 0; sample < 160; sample++ {
		deck, _ := card.Shuffle(func(n int) (int, error) { return rng.Intn(n), nil })
		hand := deck[:1+sample%12]
		if ValidateRemaining(hand) != nil {
			continue
		}
		ctx := Context{Hand: hand}
		if sample%2 == 1 {
			shape := allTemplates[rng.Intn(len(allTemplates))].Shape
			ctx.Previous = &shape
		}
		reference := map[string]bool{}
		for mask := 1; mask < 1<<len(hand); mask++ {
			var ids []card.ID
			for i, id := range hand {
				if mask&(1<<i) != 0 {
					ids = append(ids, id)
				}
			}
			options, _ := Preview(ctx, ids)
			for _, p := range options {
				reference[actionKey(p)] = true
			}
		}
		optimized := map[string]bool{}
		for _, p := range GenerateLegalMoves(ctx) {
			key := actionKey(p)
			if optimized[key] {
				t.Fatalf("duplicate action: %s", key)
			}
			if _, err := ValidatePlay(ctx, Request{p.CardIDs, &Declaration{Shape: p.Shape}}); err != nil {
				t.Fatal(err)
			}
			optimized[key] = true
		}
		if !reflect.DeepEqual(reference, optimized) || ctx.Previous == nil && len(optimized) == 0 {
			t.Fatalf("sample %d hand %v: want %v, got %v", sample, hand, reference, optimized)
		}
	}
}

func FuzzPreview(f *testing.F) {
	f.Add([]byte{4, 8, 53}, []byte{4, 8, 53})
	f.Add([]byte{0, 0, 54}, []byte{200})
	f.Fuzz(func(t *testing.T, h, s []byte) {
		hand := make([]card.ID, len(h))
		for i, b := range h {
			hand[i] = card.ID(int8(b))
		}
		selected := make([]card.ID, len(s))
		for i, b := range s {
			selected[i] = card.ID(int8(b))
		}
		options, _ := Preview(Context{Hand: hand}, selected)
		for _, p := range options {
			if _, err := ValidatePlay(Context{Hand: hand}, Request{selected, &Declaration{Shape: p.Shape}}); err != nil {
				t.Fatal(err)
			}
		}
	})
}
