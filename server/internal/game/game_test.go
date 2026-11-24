package game

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/rules"
)

func TestDealPassAndDraw(t *testing.T) {
	for n := 2; n <= 5; n++ {
		for _, mode := range []Mode{Classic, Speed} {
			for _, limit := range []int{0, 1, 2, 54} {
				t.Run(fmt.Sprintf("G-01-02-05-06-07-08/%s/%d/%d", mode, n, limit), func(t *testing.T) {
					deck := card.Deck()
					first := 1
					g, err := New(mode, n, first, deck)
					if err != nil {
						t.Fatal(err)
					}
					if g.DeckCount() != 53-5*n || g.Current() != first {
						t.Fatal("deal")
					}
					for seat := range g.hands {
						want := 5
						if seat == first {
							want++
						}
						if g.HandCount(seat) != want {
							t.Fatal(seat)
						}
					}
					if limit < g.DeckCount() {
						// Represent the already consumed prefix of the stock as public history.
						g.history = append(g.history, rules.Pattern{CardIDs: slices.Clone(g.deck[:len(g.deck)-limit])})
						g.deck = g.deck[len(g.deck)-limit:]
					}
					if _, err := g.Play(first, rules.Request{CardIDs: []card.ID{0}}); err != nil {
						t.Fatal(err)
					}
					before := slices.Clone(g.deck)
					for offset := 1; offset < n; offset++ {
						draws, err := g.Pass((first + offset) % n)
						if err != nil {
							t.Fatal(err)
						}
						if offset < n-1 {
							if len(draws) != 0 || g.Round() != 1 || g.PassCount() != offset {
								t.Fatal("early draw")
							}
							continue
						}
						count := n
						if mode == Speed {
							count = 1
						}
						if len(draws) != min(count, len(before)) {
							t.Fatal(draws)
						}
						for i, draw := range draws {
							if draw.Seat != (first+i)%n || draw.Card != before[i] {
								t.Fatal(draw)
							}
						}
					}
					if g.Phase() != Leading || g.Current() != first || g.Round() != 2 || g.PassCount() != 0 || g.LastPattern() != nil {
						t.Fatal("new round")
					}
					if err := g.CheckInventory(); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

func TestPassDoesNotForfeitRound(t *testing.T) {
	g := &Game{hands: [][]card.ID{{0, 12, 24}, {8, 16, 28}, {4, 20, 32}}, phase: Leading}
	if _, err := g.Pass(0); err != LeadingCannotPass {
		t.Fatal("S-18", err)
	}
	if _, err := g.Play(0, rules.Request{CardIDs: []card.ID{0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Pass(1); err != nil {
		t.Fatal("S-17", err)
	}
	if _, err := g.Play(2, rules.Request{CardIDs: []card.ID{4}}); err != nil {
		t.Fatal(err)
	}
	if g.PassCount() != 0 || g.Last() != 2 {
		t.Fatal("G-03")
	}
	if _, err := g.Pass(0); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Play(1, rules.Request{CardIDs: []card.ID{8}}); err != nil {
		t.Fatal("G-04", err)
	}
}

func TestAtomicWinAndRejections(t *testing.T) {
	for _, mode := range []Mode{Classic, Speed} {
		for _, phase := range []Phase{Leading, Following} {
			g := &Game{mode: mode, hands: [][]card.ID{{4, 5}, {0, 1, 8}, {12, 13, 14, 15, 20}}, deck: []card.ID{23}, phase: phase, current: 0, round: 2}
			if phase == Following {
				g.history = []rules.Pattern{{Shape: rules.Shape{Type: rules.Pair, Rank: 3, Length: 2}, CardIDs: []card.ID{2, 3}}}
			}
			before := *g
			if _, err := g.Play(0, rules.Request{CardIDs: []card.ID{99}}); err != rules.InvalidCards || !reflect.DeepEqual(before, *g) {
				t.Fatal(err)
			}
			if _, err := g.Play(0, rules.Request{CardIDs: []card.ID{4, 5}}); err != nil {
				t.Fatal(err)
			}
			want := []Result{{0, 0, 8}, {1, 3, -3}, {2, 5, -5}}
			if g.Phase() != Finished || g.Current() != -1 || g.DeckCount() != 1 || g.Round() != 2 || !slices.Equal(g.Settlement(), want) {
				t.Fatal(g)
			}
			if _, err := g.Play(0, rules.Request{CardIDs: []card.ID{4, 5}}); err != GameFinished {
				t.Fatal(err)
			}
			if _, err := g.Pass(1); err != GameFinished {
				t.Fatal(err)
			}
		}
	}
	for _, rest := range [][]card.ID{{48}, {48, 49}, {52}, {52, 53}, {48, 52}} {
		g := &Game{hands: [][]card.ID{append([]card.ID{0, 4, 8}, rest...), {1, 5}}, phase: Leading, current: 0, round: 1}
		before := *g
		before.hands = [][]card.ID{slices.Clone(g.hands[0]), slices.Clone(g.hands[1])}
		if _, err := g.Play(0, rules.Request{CardIDs: []card.ID{0, 4, 8}}); err != rules.InvalidRemaining || !reflect.DeepEqual(before, *g) {
			t.Fatal("S-01–05", err)
		}
		if _, err := g.Play(0, rules.Request{CardIDs: []card.ID{0}}); err != nil {
			t.Fatal("S-20", err)
		}
	}
}

func TestVoluntaryPassAndPrivateDrawTrigger(t *testing.T) {
	g := &Game{mode: Classic, hands: [][]card.ID{{0, 52}, {4, 8}}, notices: make([]bool, 2), deck: []card.ID{53, 40}, phase: Leading}
	if _, err := g.Play(0, rules.Request{CardIDs: []card.ID{0, 52}}); err != nil {
		t.Fatal(err)
	}
	// Use a second game where the joker stays in the leader's hand.
	g = &Game{mode: Classic, hands: [][]card.ID{{0, 12, 52}, {4, 8}}, notices: make([]bool, 2), deck: []card.ID{53, 40}, phase: Leading}
	if _, err := g.Play(0, rules.Request{CardIDs: []card.ID{0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Pass(1); err != nil || !g.Notice(0) || g.Notice(1) {
		t.Fatal("S-19/G-13", err)
	}
	deck := card.Deck()
	deck[0], deck[52] = deck[52], deck[0]
	deck[2], deck[53] = deck[53], deck[2]
	dealt, err := New(Classic, 2, 0, deck)
	if err != nil || !dealt.Notice(0) || dealt.Notice(1) {
		t.Fatal("initial notice")
	}
}
