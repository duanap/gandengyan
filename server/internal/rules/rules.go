package rules

import (
	"github.com/duanap/gandengyan/server/internal/card"
	"slices"
)

type Kind string

const (
	Single   Kind = "single"
	Pair     Kind = "pair"
	Straight Kind = "straight"
	PairRun  Kind = "pair_run"
	Bomb     Kind = "bomb"
)

type Shape struct {
	Type   Kind `json:"type"`
	Rank   int  `json:"rank"`
	Length int  `json:"length"`
}
type Wildcard struct {
	CardID card.ID `json:"cardId"`
	Rank   int     `json:"rank"`
}
type Pattern struct {
	Shape
	CardIDs   []card.ID  `json:"cardIds,omitempty"`
	Wildcards []Wildcard `json:"wildcards,omitempty"`
}
type Context struct {
	Hand     []card.ID
	Previous *Shape
}
type Request struct {
	CardIDs []card.ID    `json:"cardIds"`
	Pattern *Declaration `json:"pattern,omitempty"`
}
type Declaration struct {
	Shape
	Wildcards *[]Wildcard `json:"wildcards,omitempty"`
}

type Error string

func (e Error) Error() string { return string(e) }

const (
	InvalidCards     Error = "INVALID_CARD_IDS"
	InvalidPattern   Error = "INVALID_PATTERN"
	ChoiceRequired   Error = "PATTERN_CHOICE_REQUIRED"
	CannotBeat       Error = "CANNOT_BEAT"
	InvalidRemaining Error = "INVALID_REMAINING_SPECIALS"
	InvalidFinal     Error = "INVALID_FINAL_PLAY"
)

type template struct {
	Shape
	groups  int
	perRank int
}

func templates() []template {
	var all []template
	for rank := card.Three; rank <= card.Two; rank++ {
		all = append(all, template{Shape{Single, rank, 1}, 1, 1}, template{Shape{Pair, rank, 2}, 1, 2})
		for length := 3; length <= 6; length++ {
			all = append(all, template{Shape{Bomb, rank, length}, 1, length})
		}
		for groups := 2; rank+groups-1 <= card.Ace; groups++ {
			all = append(all, template{Shape{PairRun, rank, groups * 2}, groups, 2})
			if groups >= 3 {
				all = append(all, template{Shape{Straight, rank, groups}, groups, 1})
			}
		}
	}
	return all
}

var allTemplates = templates()

type inventory struct {
	naturals [16][]card.ID
	jokers   []card.ID
	mask     uint64
	ordinary int // 3 through A
}

func collect(ids []card.ID) (inventory, bool) {
	var inv inventory
	for _, id := range ids {
		if !id.Valid() || inv.mask&(1<<id) != 0 {
			return inventory{}, false
		}
		inv.mask |= 1 << id
		if rank := id.Rank(); rank != 0 {
			inv.naturals[rank] = append(inv.naturals[rank], id)
			if rank != card.Two {
				inv.ordinary++
			}
		} else {
			inv.jokers = append(inv.jokers, id)
		}
	}
	for rank := card.Three; rank <= card.Two; rank++ {
		slices.Sort(inv.naturals[rank])
	}
	slices.Sort(inv.jokers)
	return inv, true
}

func ResolvePatterns(selected []card.ID) []Pattern {
	inv, ok := collect(selected)
	if !ok || len(selected) == len(inv.jokers) {
		return nil
	}
	var patterns []Pattern
	for _, t := range allTemplates {
		if t.Length != len(selected) {
			continue
		}
		var missing []int
		valid := true
		for rank := card.Three; rank <= card.Two; rank++ {
			want := 0
			if rank >= t.Rank && rank < t.Rank+t.groups {
				want = t.perRank
			}
			have := len(inv.naturals[rank])
			if have > want {
				valid = false
				break
			}
			for i := have; i < want; i++ {
				missing = append(missing, rank)
			}
		}
		if valid && len(missing) == len(inv.jokers) {
			patterns = append(patterns, makePattern(t.Shape, selected, inv.jokers, missing))
		}
	}
	return patterns
}

func makePattern(shape Shape, ids, jokers []card.ID, missing []int) Pattern {
	wildcards := make([]Wildcard, 0, len(missing))
	for i, rank := range missing {
		wildcards = append(wildcards, Wildcard{jokers[i], rank})
	}
	return Pattern{shape, card.Sorted(ids), wildcards}
}

func CanBeat(candidate, previous Shape) bool {
	if candidate.Type == Bomb {
		return previous.Type != Bomb || candidate.Length > previous.Length ||
			candidate.Length == previous.Length && candidate.Rank > previous.Rank
	}
	if candidate.Type != previous.Type || candidate.Length != previous.Length {
		return false
	}
	if candidate.Type == Single || candidate.Type == Pair {
		return previous.Rank < card.Two && (candidate.Rank == card.Two || candidate.Rank == previous.Rank+1)
	}
	return candidate.Rank == previous.Rank+1
}

func ValidateRemaining(remaining []card.ID) error {
	if len(remaining) == 0 || len(remaining) > 2 {
		return nil
	}
	for _, id := range remaining {
		if rank := id.Rank(); rank >= card.Three && rank <= card.Ace {
			return nil
		}
	}
	return InvalidRemaining
}

func validateOutcome(shape Shape, remaining, ordinary int) error {
	if remaining == 0 && shape.Rank == card.Two && shape.Length <= 2 {
		return InvalidFinal
	}
	if remaining > 0 && remaining <= 2 && ordinary == 0 {
		return InvalidRemaining
	}
	return nil
}

func Preview(ctx Context, ids []card.ID) ([]Pattern, error) {
	hand, ok := collect(ctx.Hand)
	selected, valid := collect(ids)
	if !ok || !valid || len(ids) == 0 || selected.mask&^hand.mask != 0 {
		return nil, InvalidCards
	}
	patterns := ResolvePatterns(ids)
	if len(patterns) == 0 {
		return nil, InvalidPattern
	}
	var legal []Pattern
	var rejection error = CannotBeat
	for _, pattern := range patterns {
		if ctx.Previous != nil && !CanBeat(pattern.Shape, *ctx.Previous) {
			continue
		}
		if err := validateOutcome(pattern.Shape, len(ctx.Hand)-len(ids), hand.ordinary-selected.ordinary); err != nil {
			rejection = err
			continue
		}
		legal = append(legal, pattern)
	}
	if len(legal) == 0 {
		return nil, rejection
	}
	return legal, nil
}

func ValidatePlay(ctx Context, request Request) (Pattern, error) {
	patterns, err := Preview(ctx, request.CardIDs)
	if err != nil {
		return Pattern{}, err
	}
	if request.Pattern == nil {
		if len(patterns) != 1 {
			return Pattern{}, ChoiceRequired
		}
		return patterns[0], nil
	}
	for _, candidate := range patterns {
		if candidate.Shape == request.Pattern.Shape && (request.Pattern.Wildcards == nil || slices.Equal(candidate.Wildcards, *request.Pattern.Wildcards)) {
			return candidate, nil
		}
	}
	return Pattern{}, InvalidPattern
}

// GenerateLegalMoves collapses equivalent suits and joker identities, while
// keeping choices that consume different ranks or numbers of natural cards.
func GenerateLegalMoves(ctx Context) []Pattern {
	inv, ok := collect(ctx.Hand)
	if !ok {
		return nil
	}
	var moves []Pattern
	for _, t := range allTemplates {
		if t.Length > len(ctx.Hand) || ctx.Previous != nil && !CanBeat(t.Shape, *ctx.Previous) {
			continue
		}
		var minimum [13]int
		for group := t.groups - 1; group >= 0; group-- {
			minimum[group] = minimum[group+1] + max(0, t.perRank-len(inv.naturals[t.Rank+group]))
		}
		if minimum[0] > len(inv.jokers) {
			continue
		}
		var selected [24]card.ID
		var missing [2]int
		var search func(int, int, int)
		search = func(group, used, wild int) {
			if group == t.groups {
				if used == 0 {
					return
				}
				ordinary := inv.ordinary
				if t.Rank != card.Two {
					ordinary -= used
				}
				if validateOutcome(t.Shape, len(ctx.Hand)-t.Length, ordinary) != nil {
					return
				}
				copy(selected[used:], inv.jokers[:wild])
				moves = append(moves, makePattern(t.Shape, selected[:used+wild], inv.jokers, missing[:wild]))
				return
			}
			rank := t.Rank + group
			for take := min(t.perRank, len(inv.naturals[rank])); take >= 0; take-- {
				need := t.perRank - take
				if wild+need+minimum[group+1] > len(inv.jokers) {
					break
				}
				copy(selected[used:], inv.naturals[rank][:take])
				for j := 0; j < need; j++ {
					missing[wild+j] = rank
				}
				search(group+1, used+take, wild+need)
			}
		}
		search(0, 0, 0)
	}
	return moves
}

func CanFinish(ctx Context) bool {
	_, err := Preview(ctx, ctx.Hand)
	return err == nil
}
