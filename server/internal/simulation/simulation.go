package simulation

import (
	"fmt"
	"math/rand"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/rules"
)

type Record struct {
	Seed      int64     `json:"seed"`
	Players   int       `json:"players"`
	Mode      game.Mode `json:"mode"`
	Actions   int       `json:"actions"`
	Rounds    int       `json:"rounds"`
	Winner    int       `json:"winnerSeat"`
	Depleted  bool      `json:"deckDepleted"`
	Bombs     int       `json:"bombs"`
	Passes    int       `json:"passes"`
	Wildcards int       `json:"wildcards"`
	Error     string    `json:"error,omitempty"`
}

func Run(seed int64, n int, mode game.Mode) (rec Record, err error) {
	rec = Record{Seed: seed, Players: n, Mode: mode, Winner: -1}
	defer func() {
		if err != nil {
			rec.Error = err.Error()
		}
	}()
	rng := rand.New(rand.NewSource(seed))
	deck, err := card.Shuffle(func(n int) (int, error) { return rng.Intn(n), nil })
	if err != nil {
		return rec, err
	}
	g, err := game.New(mode, n, rng.Intn(n), deck)
	if err != nil {
		return rec, err
	}
	if err = g.CheckInventory(); err != nil {
		return rec, err
	}
	for g.Phase() != game.Finished {
		if rec.Actions >= 54*n {
			return rec, fmt.Errorf("action limit exceeded")
		}
		actor := g.Current()
		ctx, ctxErr := g.Context(actor)
		if ctxErr != nil {
			return rec, ctxErr
		}
		moves := rules.GenerateLegalMoves(ctx)
		if len(moves) == 0 && ctx.Previous == nil {
			return rec, fmt.Errorf("no legal lead with %v", ctx.Hand)
		}
		if len(moves) == 0 || ctx.Previous != nil && rng.Intn(5) == 0 && !rules.CanFinish(ctx) {
			_, err = g.Pass(actor)
			rec.Passes++
		} else {
			best := moves[rng.Intn(len(moves))]
			if (actor+int(seed%2))%2 == 0 {
				for _, m := range moves {
					if m.Length > best.Length || m.Length == best.Length && len(m.Wildcards) < len(best.Wildcards) {
						best = m
					}
				}
			}
			_, err = g.Play(actor, rules.Request{CardIDs: best.CardIDs, Pattern: &rules.Declaration{Shape: best.Shape}})
			if best.Type == rules.Bomb {
				rec.Bombs++
			}
			rec.Wildcards += len(best.Wildcards)
		}
		rec.Actions++
		if err != nil {
			return rec, err
		}
		if err = g.CheckInventory(); err != nil {
			return rec, err
		}
	}
	rec.Winner = g.Last()
	rec.Rounds = g.Round()
	rec.Depleted = g.DeckCount() == 0
	return rec, nil
}
