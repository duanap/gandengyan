package game

import (
	"errors"
	"slices"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/rules"
)

type Mode string

const (
	Classic Mode = "classic"
	Speed   Mode = "speed"
)

func (m Mode) Valid() bool { return m == Classic || m == Speed }

type Phase string

const (
	Waiting   Phase = "waiting"
	Leading   Phase = "leading"
	Following Phase = "following"
	Finished  Phase = "finished"
)

const (
	NotYourTurn       rules.Error = "NOT_YOUR_TURN"
	GameFinished      rules.Error = "GAME_FINISHED"
	LeadingCannotPass rules.Error = "LEADING_CANNOT_PASS"
)

type Draw struct {
	Seat int
	Card card.ID
}

type Result struct {
	Seat      int
	HandCount int
	Delta     int
}

type Game struct {
	mode       Mode
	hands      [][]card.ID
	deck       []card.ID
	history    []rules.Pattern
	notices    []bool
	current    int
	leader     int
	last       int
	passCount  int
	round      int
	phase      Phase
	settlement []Result
}

func New(mode Mode, players, first int, deck []card.ID) (*Game, error) {
	if !mode.Valid() || players < 2 || players > 5 || first < 0 || first >= players || len(deck) != card.Count {
		return nil, errors.New("invalid deal")
	}
	var seen uint64
	for _, id := range deck {
		if !id.Valid() || seen&(1<<id) != 0 {
			return nil, errors.New("invalid deck")
		}
		seen |= 1 << id
	}
	g := &Game{mode: mode, hands: make([][]card.ID, players), deck: slices.Clone(deck), notices: make([]bool, players), current: first, leader: first, last: -1, round: 1, phase: Leading}
	for i := 0; i < 5; i++ {
		for offset := 0; offset < players; offset++ {
			g.draw((first + offset) % players)
		}
	}
	g.draw(first)
	return g, nil
}

func (g *Game) draw(seat int) card.ID {
	id := g.deck[0]
	g.deck = g.deck[1:]
	g.hands[seat] = append(g.hands[seat], id)
	if !g.notices[seat] && slices.Contains(g.hands[seat], card.SmallJoker) && slices.Contains(g.hands[seat], card.BigJoker) {
		g.notices[seat] = true
	}
	return id
}

func (g *Game) Phase() Phase            { return g.phase }
func (g *Game) Current() int            { return g.current }
func (g *Game) Leader() int             { return g.leader }
func (g *Game) Last() int               { return g.last }
func (g *Game) Round() int              { return g.round }
func (g *Game) PassCount() int          { return g.passCount }
func (g *Game) DeckCount() int          { return len(g.deck) }
func (g *Game) HandCount(seat int) int  { return len(g.hands[seat]) }
func (g *Game) Hand(seat int) []card.ID { return card.Sorted(g.hands[seat]) }
func (g *Game) Notice(seat int) bool    { return g.notices[seat] }
func (g *Game) Settlement() []Result    { return slices.Clone(g.settlement) }
func (g *Game) History() []rules.Pattern {
	out := make([]rules.Pattern, len(g.history))
	for i, p := range g.history {
		out[i] = clonePattern(p)
	}
	return out
}
func clonePattern(p rules.Pattern) rules.Pattern {
	p.CardIDs = slices.Clone(p.CardIDs)
	p.Wildcards = slices.Clone(p.Wildcards)
	return p
}
func (g *Game) LastPattern() *rules.Pattern {
	if g.phase != Following {
		return nil
	}
	p := clonePattern(g.history[len(g.history)-1])
	return &p
}

func (g *Game) Context(seat int) (rules.Context, error) {
	if g.phase == Finished {
		return rules.Context{}, GameFinished
	}
	if seat != g.current {
		return rules.Context{}, NotYourTurn
	}
	ctx := rules.Context{Hand: g.Hand(seat)}
	if g.phase == Following {
		shape := g.history[len(g.history)-1].Shape
		ctx.Previous = &shape
	}
	return ctx, nil
}

func (g *Game) Play(seat int, req rules.Request) (rules.Pattern, error) {
	ctx, err := g.Context(seat)
	if err != nil {
		return rules.Pattern{}, err
	}
	p, err := rules.ValidatePlay(ctx, req)
	if err != nil {
		return rules.Pattern{}, err
	}
	var used uint64
	for _, id := range p.CardIDs {
		used |= 1 << id
	}
	g.hands[seat] = slices.DeleteFunc(g.hands[seat], func(id card.ID) bool { return used&(1<<id) != 0 })
	g.history = append(g.history, clonePattern(p))
	g.last = seat
	g.passCount = 0
	if len(g.hands[seat]) == 0 {
		g.finish(seat)
		return p, nil
	}
	g.phase = Following
	g.current = (seat + 1) % len(g.hands)
	return p, nil
}

func (g *Game) Pass(seat int) ([]Draw, error) {
	if _, err := g.Context(seat); err != nil {
		return nil, err
	}
	if g.phase != Following {
		return nil, LeadingCannotPass
	}
	g.passCount++
	if g.passCount < len(g.hands)-1 {
		g.current = (seat + 1) % len(g.hands)
		return nil, nil
	}
	count := 1
	if g.mode == Classic {
		count = len(g.hands)
	}
	draws := make([]Draw, 0, min(count, len(g.deck)))
	for i := 0; i < count && len(g.deck) > 0; i++ {
		target := (g.last + i) % len(g.hands)
		draws = append(draws, Draw{target, g.draw(target)})
	}
	g.current, g.leader = g.last, g.last
	g.passCount = 0
	g.round++
	g.phase = Leading
	return draws, nil
}

func (g *Game) finish(winner int) {
	g.phase = Finished
	g.current = -1
	g.settlement = make([]Result, len(g.hands))
	for seat, hand := range g.hands {
		g.settlement[seat] = Result{seat, len(hand), -len(hand)}
	}
	for seat, result := range g.settlement {
		if seat != winner {
			g.settlement[winner].Delta -= result.Delta
		}
	}
}

// CheckInventory checks the same physical inventory used by the state machine.
func (g *Game) CheckInventory() error {
	var seen uint64
	add := func(ids []card.ID) bool {
		for _, id := range ids {
			if !id.Valid() || seen&(1<<id) != 0 {
				return false
			}
			seen |= 1 << id
		}
		return true
	}
	if !add(g.deck) {
		return errors.New("invalid deck inventory")
	}
	empty := 0
	for _, hand := range g.hands {
		if len(hand) == 0 {
			empty++
		}
		if !add(hand) || rules.ValidateRemaining(hand) != nil {
			return errors.New("invalid hand inventory")
		}
	}
	for _, p := range g.history {
		if !add(p.CardIDs) {
			return errors.New("invalid played inventory")
		}
	}
	if seen != (1<<card.Count)-1 {
		return errors.New("missing cards")
	}
	if g.phase == Finished {
		total := 0
		for _, result := range g.settlement {
			total += result.Delta
		}
		if empty != 1 || total != 0 || g.current != -1 {
			return errors.New("invalid settlement")
		}
	} else if empty != 0 || g.current < 0 || g.passCount >= len(g.hands)-1 {
		return errors.New("invalid active game")
	}
	return nil
}
