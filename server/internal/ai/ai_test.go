package ai

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/rules"
)

func TestImmediateWinsAndFallback(t *testing.T) {
	for _, tc := range []struct {
		hand     []card.ID
		previous *rules.Shape
	}{
		{[]card.ID{48, 49, 50, 51, 52, 53}, &rules.Shape{Type: rules.Bomb, Rank: 14, Length: 6}},
		{[]card.ID{4, 8, 53}, &rules.Shape{Type: rules.Straight, Rank: 3, Length: 3}},
		{[]card.ID{0, 52, 53}, nil},
		{[]card.ID{0, 4, 48}, &rules.Shape{Type: rules.Single, Rank: 12, Length: 1}},
		{[]card.ID{0, 4, 48, 52}, nil},
	} {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		p := Choose(ctx, View{Hand: tc.hand, Previous: tc.previous, Mode: game.Speed, DeckCount: 10, Opponents: []Opponent{{HandCount: 2}}})
		rctx := rules.Context{Hand: tc.hand, Previous: tc.previous}
		if p.Move == nil {
			if tc.previous == nil || rules.CanFinish(rctx) {
				t.Fatalf("unexpected pass: %+v", tc)
			}
			continue
		}
		m := p.Move
		if _, err := rules.ValidatePlay(rctx, rules.Request{CardIDs: m.CardIDs, Pattern: &rules.Declaration{Shape: m.Shape}}); err != nil {
			t.Fatal(err)
		}
		if rules.CanFinish(rctx) && m.Length != len(tc.hand) {
			t.Fatal("missed immediate win")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if p := Choose(ctx, View{Hand: []card.ID{0, 17, 24, 33, 44, 53}, Mode: game.Classic}); p.Move == nil || !p.Limited {
		t.Fatal("cancelled search did not return legal fallback")
	}
}

func TestPolicyTournament(t *testing.T) {
	if os.Getenv("GDY_AI_TOURNAMENT") == "" {
		t.Skip("enable comprehensive AI comparison with GDY_AI_TOURNAMENT=1")
	}
	const deals = 320
	for n := 2; n <= 5; n++ {
		for _, mode := range []game.Mode{game.Classic, game.Speed} {
			for _, opponent := range []string{"random", "greedy"} {
				wins, score, bounded, slow := 0, 0, 0, 0
				var latency []time.Duration
				for i := 0; i < deals; i++ {
					result := playTournament(t, n, mode, int64(23001+i), i%n, opponent)
					wins += result.won
					score += result.score
					bounded += result.bounded
					slow += result.slow
					latency = append(latency, result.durations...)
				}
				slices.Sort(latency)
				t.Logf("%d %s vs %s: wins=%d/%d (%.1f%%) avg_delta=%.2f decisions=%d p50=%s p95=%s p99=%s max=%s search_limited=%d over_100ms=%d",
					n, mode, opponent, wins, deals, 100*float64(wins)/deals, float64(score)/deals, len(latency), latency[len(latency)/2], latency[len(latency)*95/100], latency[len(latency)*99/100],
					latency[len(latency)-1], bounded, slow)
			}
		}
	}
}

type match struct {
	won, score, bounded, slow int
	durations                 []time.Duration
}

func playTournament(t *testing.T, n int, mode game.Mode, seed int64, aiSeat int, opponent string) match {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	deck, _ := card.Shuffle(func(n int) (int, error) { return rng.Intn(n), nil })
	g, err := game.New(mode, n, rng.Intn(n), deck)
	if err != nil {
		t.Fatal(err)
	}
	result := match{}
	for action := 0; g.Phase() != game.Finished; action++ {
		if action >= 54*n {
			t.Fatalf("seed %d: game did not terminate", seed)
		}
		seat := g.Current()
		ctx, _ := g.Context(seat)
		var move *rules.Pattern
		if seat == aiSeat {
			v := View{Hand: ctx.Hand, Previous: ctx.Previous, Mode: mode, DeckCount: g.DeckCount(), PassCount: g.PassCount(), Round: g.Round()}
			for _, p := range g.History() {
				v.Played = append(v.Played, p.CardIDs...)
			}
			for offset := 1; offset < n; offset++ {
				v.Opponents = append(v.Opponents, Opponent{HandCount: g.HandCount((seat + offset) % n)})
			}
			start := time.Now()
			deadline, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			choice := Choose(deadline, v)
			cancel()
			elapsed := time.Since(start)
			if choice.Limited {
				result.bounded++
			}
			if elapsed > 100*time.Millisecond {
				result.slow++
			}
			result.durations = append(result.durations, elapsed)
			move = choice.Move
		} else {
			options := rules.GenerateLegalMoves(ctx)
			if len(options) > 0 {
				if opponent == "greedy" {
					best := options[0]
					for _, candidate := range options {
						if candidate.Length > best.Length || candidate.Length == best.Length && len(candidate.Wildcards) < len(best.Wildcards) {
							best = candidate
						}
					}
					move = &best
				} else if ctx.Previous == nil || rng.Intn(len(options)+1) < len(options) {
					move = &options[rng.Intn(len(options))]
				}
			}
		}
		if move == nil {
			_, err = g.Pass(seat)
		} else {
			_, err = g.Play(seat, rules.Request{CardIDs: move.CardIDs, Pattern: &rules.Declaration{Shape: move.Shape}})
		}
		if err != nil {
			t.Fatal(fmt.Sprintf("seed %d mode %s action %d", seed, mode, action), err)
		}
		if err := g.CheckInventory(); err != nil {
			t.Fatal(err)
		}
	}
	if g.Last() == aiSeat {
		result.won = 1
	}
	result.score = g.Settlement()[aiSeat].Delta
	return result
}
