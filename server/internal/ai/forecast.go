package ai

import (
	"context"
	"math/rand/v2"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/rules"
)

type rated struct {
	move  *rules.Pattern
	score float64
}

func forecast(ctx context.Context, v View, ranked []rated) (*rules.Pattern, bool) {
	if len(ranked) <= 1 || len(v.Opponents) == 0 {
		return ranked[0].move, false
	}
	ranked = ranked[:min(len(ranked), 6)]
	var mask, seed uint64
	for _, id := range v.Hand {
		mask |= 1 << id
	}
	seed = mask
	for _, id := range v.Played {
		mask |= 1 << id
	}
	unknown := make([]card.ID, 0, card.Count-len(v.Hand)-len(v.Played))
	for _, id := range card.Deck() {
		if mask&(1<<id) == 0 {
			unknown = append(unknown, id)
		}
	}
	count := v.DeckCount
	for _, op := range v.Opponents {
		count += op.HandCount
	}
	if count != len(unknown) {
		return ranked[0].move, false
	}
	rng := rand.New(rand.NewPCG(seed, mask+uint64(v.Round)))
	sums := make([]float64, len(ranked))
	const samples = 5
	for s := 0; s < samples; s++ {
		if ctx.Err() != nil {
			return ranked[0].move, true
		}
		rng.Shuffle(len(unknown), func(i, j int) { unknown[i], unknown[j] = unknown[j], unknown[i] })
		hands := make([][]card.ID, len(v.Opponents)+1)
		hands[0] = v.Hand
		offset := 0
		for i, op := range v.Opponents {
			hands[i+1] = unknown[offset : offset+op.HandCount]
			offset += op.HandCount
		}
		for i, option := range ranked {
			if ctx.Err() != nil {
				return ranked[0].move, true
			}
			now := game.FromHypothesis(v.Mode, hands, unknown[offset:], v.Played, v.Previous, v.PassCount, v.Round)
			sums[i] += evaluate(now, option.move, len(hands))
		}
	}
	choice := ranked[0].move
	best := -1e9
	for i, c := range ranked {
		score := c.score + 1.15*sums[i]/samples
		if score > best {
			choice, best = c.move, score
		}
	}
	return choice, false
}

func evaluate(g *game.Game, m *rules.Pattern, n int) float64 {
	apply(g, m)
	// Follow one complete table rotation and let a regained lead play once.
	for turns := 0; turns <= n && g.Phase() != game.Finished; turns++ {
		ctx, _ := g.Context(g.Current())
		var best *rules.Pattern
		bestScore := -1e9
		options := rules.GenerateLegalMoves(ctx)
		for i := range options {
			p := &options[i]
			if p.Length == len(ctx.Hand) {
				best = p
				break
			}
			score := float64(p.Length)*2 - float64(len(p.Wildcards))*.32
			if p.Type == rules.Bomb {
				score -= 0.7
			}
			if p.Type == rules.Single && p.Rank == card.Two {
				score += 0.4
			}
			if score > bestScore {
				best, bestScore = p, score
			}
		}
		apply(g, best)
	}
	if g.Phase() == game.Finished {
		if g.Last() == 0 {
			return 9
		}
		return -9
	}
	foes, low := 0, 54
	for seat := 1; seat < n; seat++ {
		left := g.HandCount(seat)
		foes += left
		low = min(low, left)
	}
	score := -0.67*float64(g.HandCount(0)) + 0.38*float64(foes)/float64(n-1) + 0.32*float64(low)
	if g.Last() == 0 {
		score += 0.8
	}
	if g.Current() == 0 {
		score += 0.35
	}
	return score
}

func apply(g *game.Game, m *rules.Pattern) {
	if m == nil {
		_, _ = g.Pass(g.Current())
	} else {
		_, _ = g.Play(g.Current(), rules.Request{CardIDs: m.CardIDs, Pattern: &rules.Declaration{Shape: m.Shape}})
	}
}
