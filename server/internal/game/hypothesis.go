package game

import (
	"slices"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/rules"
)

// FromHypothesis creates an independent position from simulated hands and
// stock. Seat zero is the decision maker and acts first in this position.
func FromHypothesis(mode Mode, hands [][]card.ID, deck, played []card.ID, previous *rules.Shape, passCount, round int) *Game {
	g := &Game{
		mode: mode, hands: make([][]card.ID, len(hands)),
		deck: slices.Clone(deck), notices: make([]bool, len(hands)),
		round: round, phase: Leading, last: -1,
	}
	for i, hand := range hands {
		g.hands[i] = slices.Clone(hand)
	}
	if len(played) != 0 {
		pattern := rules.Pattern{CardIDs: slices.Clone(played)}
		if previous != nil {
			pattern.Shape = *previous
		}
		g.history = append(g.history, pattern)
	}
	if previous != nil {
		g.phase = Following
		g.last = len(hands) - passCount - 1
		g.passCount = passCount
	}
	return g
}
