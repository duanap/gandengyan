// Package ai consumes a private player view and public table data only.
package ai

import (
	"context"
	"slices"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/rules"
)

type Opponent struct {
	HandCount int
	Score     int
}

// Opponents are in action order, starting with the next seat.
type View struct {
	Hand      []card.ID
	Previous  *rules.Shape
	Played    []card.ID
	Opponents []Opponent
	Mode      game.Mode
	DeckCount int
	Round     int
	PassCount int
}
type Decision struct {
	Move    *rules.Pattern
	Limited bool
}

type candidate struct {
	move *rules.Pattern
	base float64
	rest []card.ID
}

func Choose(ctx context.Context, v View) Decision {
	moves := rules.GenerateLegalMoves(rules.Context{Hand: v.Hand, Previous: v.Previous})
	if len(moves) == 0 {
		return Decision{}
	}
	for i := range moves {
		if moves[i].Length == len(v.Hand) {
			return Decision{Move: &moves[i]}
		}
	}
	remaining := unseen(v)
	urgent := false
	for _, opponent := range v.Opponents {
		urgent = urgent || opponent.HandCount <= 2
	}
	options := make([]candidate, 0, len(moves)+1)
	for i := range moves {
		move := &moves[i]
		rest := without(v.Hand, move.CardIDs)
		score := float64(move.Length)*0.9 + shapeValue(rest) - shapeValue(v.Hand)
		score -= float64(len(move.Wildcards)) * 0.95
		control := controlValue(*move, remaining)
		score += control * 1.15
		if v.Mode == game.Speed && v.DeckCount > 0 {
			score -= control * 0.55
		}
		if move.Type == rules.Bomb {
			score -= 0.7
			if urgent {
				score += 1.9 * control
			}
		}
		if len(rest) == 1 {
			score += 2.4 * control
		}
		if len(rest) <= 8 && rules.CanFinish(rules.Context{Hand: rest}) {
			score += 2.4 + control
		}
		if v.Previous == nil && urgent {
			score += control * 1.8
		}
		if v.Previous == nil && !urgent && move.Type == rules.Single && move.Rank != card.Two {
			score -= float64(move.Rank-card.Three) * 0.09
		}
		if urgent && len(v.Opponents) > 0 && canWinOnSingle(v.Opponents[0], move.Shape, remaining) {
			score -= 4.2
		}
		options = append(options, candidate{move, score, rest})
	}
	if v.Previous != nil {
		score := 0.0
		if urgent {
			score -= 1.7
		}
		if v.Mode == game.Speed && v.DeckCount > 0 {
			score += 0.45
		}
		if v.PassCount == len(v.Opponents)-1 && urgent {
			score -= 0.7
		}
		if len(v.Opponents) > 0 && canWinOnSingle(v.Opponents[0], *v.Previous, remaining) {
			score -= 4.2
		}
		options = append(options, candidate{nil, score, v.Hand})
	}
	slices.SortStableFunc(options, func(a, b candidate) int {
		if a.base > b.base {
			return -1
		}
		if a.base < b.base {
			return 1
		}
		return 0
	})
	fallback := options[0].move
	if ctx.Err() != nil {
		return Decision{fallback, true}
	}
	// The fixed node cap makes a completed search deterministic. A deadline
	// under contention can still return the complete heuristic decision.
	search := planner{ctx: ctx, nodes: 180, memo: make(map[uint64]int)}
	ranked := make([]rated, 0, min(10, len(options)))
	for _, c := range options[:min(10, len(options))] {
		cost := search.turns(c.rest)
		score := c.base - float64(cost)*1.25
		ranked = append(ranked, rated{c.move, score})
	}
	if ctx.Err() != nil {
		return Decision{fallback, true}
	}
	slices.SortStableFunc(ranked, func(a, b rated) int {
		if a.score > b.score {
			return -1
		}
		if a.score < b.score {
			return 1
		}
		return 0
	})
	choice, interrupted := forecast(ctx, v, ranked)
	return Decision{choice, interrupted || search.nodes == 0}
}

func without(hand, ids []card.ID) []card.ID {
	var mask uint64
	for _, id := range ids {
		mask |= 1 << id
	}
	out := make([]card.ID, 0, len(hand)-len(ids))
	for _, id := range hand {
		if mask&(1<<id) == 0 {
			out = append(out, id)
		}
	}
	return out
}

func unseen(v View) [16]int {
	var counts [16]int
	for r := card.Three; r <= card.Two; r++ {
		counts[r] = 4
	}
	counts[0] = 2
	for _, id := range v.Hand {
		counts[id.Rank()]--
	}
	for _, id := range v.Played {
		counts[id.Rank()]--
	}
	return counts
}
func controlValue(p rules.Pattern, unseen [16]int) float64 {
	if p.Type == rules.Bomb {
		return 0.79 + 0.055*float64(p.Length-3) + 0.003*float64(p.Rank-card.Three)
	}
	if p.Rank == card.Two {
		return 0.82
	}
	next := p.Rank + 1
	switch p.Type {
	case rules.Single:
		return 0.67 - 0.095*float64(unseen[next]+unseen[card.Two])
	case rules.Pair:
		return 0.72 - 0.065*float64(unseen[next]+unseen[card.Two]+unseen[0])
	case rules.Straight:
		if next+p.Length-1 > card.Ace {
			return 0.86
		}
		return runControl(unseen, next, p.Length, 1)
	case rules.PairRun:
		if next+p.Length/2-1 > card.Ace {
			return 0.86
		}
		return runControl(unseen, next, p.Length/2, 2)
	}
	return 0
}
func runControl(unseen [16]int, start, groups, required int) float64 {
	shortage := 0
	for r := start; r < start+groups; r++ {
		shortage += max(0, required-unseen[r])
	}
	if shortage > unseen[0] {
		return 0.9
	}
	return min(0.83, 0.48+0.035*float64(groups)+0.1*float64(shortage))
}
func canWinOnSingle(next Opponent, p rules.Shape, unseen [16]int) bool {
	return next.HandCount == 1 && p.Type == rules.Single && p.Rank < card.Ace && unseen[p.Rank+1] > 0
}

func shapeValue(hand []card.ID) float64 {
	var count [16]int
	for _, id := range hand {
		count[id.Rank()]++
	}
	value := 0.0
	streak := 0
	for rank := card.Three; rank <= card.Two; rank++ {
		n := count[rank]
		if n >= 2 {
			value += 0.26
		}
		if n >= 3 {
			value += 0.62
		}
		if n >= 4 {
			value += 0.4
		}
		if rank != card.Two && n >= 1 {
			streak++
			if streak == 3 {
				value += 0.55
			}
			if streak > 3 {
				value += 0.38
			}
		} else {
			streak = 0
		}
	}
	return value
}

type planner struct {
	ctx   context.Context
	nodes int
	memo  map[uint64]int
}

func (p *planner) turns(hand []card.ID) int {
	if len(hand) <= 1 {
		return len(hand)
	}
	estimate := max(2, (len(hand)+1)/2)
	if len(hand) > 10 || p.nodes == 0 || p.ctx.Err() != nil {
		return estimate
	}
	var key uint64
	for _, id := range hand {
		key |= 1 << id
	}
	if count, ok := p.memo[key]; ok {
		return count
	}
	p.nodes--
	if rules.CanFinish(rules.Context{Hand: hand}) {
		p.memo[key] = 1
		return 1
	}
	moves := rules.GenerateLegalMoves(rules.Context{Hand: hand})
	best := len(hand)
	slices.SortStableFunc(moves, func(a, b rules.Pattern) int { return b.Length - a.Length })
	before := p.nodes
	for _, move := range moves {
		best = min(best, 1+p.turns(without(hand, move.CardIDs)))
		if best == 2 {
			p.memo[key] = best
			return best
		}
		if p.nodes == 0 || p.ctx.Err() != nil {
			break
		}
	}
	if p.nodes > 0 && p.ctx.Err() == nil && before > 0 {
		p.memo[key] = best
	}
	return best
}
