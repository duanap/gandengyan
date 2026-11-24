package room

import (
	"context"
	"log"
	"time"

	"github.com/duanap/gandengyan/server/internal/ai"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/protocol"
	"github.com/duanap/gandengyan/server/internal/rules"
)

func (r *Room) botView(seat int) ai.View {
	g := r.game
	ctx, _ := g.Context(seat)
	view := ai.View{Hand: ctx.Hand, Previous: ctx.Previous, DeckCount: g.DeckCount(), Mode: r.mode, PassCount: g.PassCount(), Round: g.Round()}
	for offset := 1; offset < r.capacity; offset++ {
		i := (seat + offset) % r.capacity
		view.Opponents = append(view.Opponents, ai.Opponent{HandCount: g.HandCount(i), Score: r.players[i].score})
	}
	for _, play := range g.History() {
		view.Played = append(view.Played, play.CardIDs...)
	}
	return view
}

func (r *Room) cancelAI() {
	if r.aiCancel != nil {
		r.aiCancel()
		r.aiCancel = nil
	}
}

func (r *Room) scheduleAI() {
	if r.aiCancel != nil && r.aiRevision == r.revision {
		return
	}
	r.cancelAI()
	if r.game == nil || r.game.Phase() == game.Finished || !r.offlineSince.IsZero() {
		return
	}
	seat := r.game.Current()
	if !r.players[seat].bot {
		return
	}
	id, revision := r.gameID, r.revision
	view := r.botView(seat)
	ctx, cancel := context.WithCancel(r.ctx)
	r.aiCancel, r.aiRevision = cancel, revision
	r.dir.aiWG.Add(1)
	go func() {
		defer r.dir.aiWG.Done()
		delay := time.NewTimer(430 * time.Millisecond)
		defer delay.Stop()
		select {
		case <-ctx.Done():
			return
		case r.dir.workers <- struct{}{}:
		}
		limit, stop := context.WithTimeout(ctx, 100*time.Millisecond)
		decision := ai.Choose(limit, view)
		stop()
		<-r.dir.workers
		select {
		case <-ctx.Done():
			return
		case <-delay.C:
		}
		r.post(func() {
			if ctx.Err() != nil || r.gameID != id || r.revision != revision || r.game.Current() != seat {
				return
			}
			var events []protocol.Event
			var err error
			if decision.Move == nil {
				events, err = r.applyPass(r.players[seat])
			} else {
				p := decision.Move
				events, err = r.applyPlay(r.players[seat], rules.Request{CardIDs: p.CardIDs, Pattern: &rules.Declaration{Shape: p.Shape}})
			}
			if err != nil {
				log.Printf("robot action rejected room=%s game=%s rev=%d code=%v", r.id, id, revision, err)
				return
			}
			r.revision++
			r.broadcast(events)
		})
	}()
}
