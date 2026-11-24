package room

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/duanap/gandengyan/server/internal/ai"
	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/protocol"
	"github.com/duanap/gandengyan/server/internal/session"
)

func testConfig() Config {
	return Config{MaxRooms: 16, OfflineTTL: time.Minute, SessionTTL: time.Hour, AIWorkers: 1}
}
func expectMessage(t *testing.T, p *session.Peer, kind string) protocol.Message {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case m := <-p.Out:
			if m.Type == kind {
				return m
			}
		case <-deadline:
			t.Fatal("missing message", kind)
		}
	}
}
func expectClose(t *testing.T, p *session.Peer, code int) {
	t.Helper()
	select {
	case actual := <-p.Stop:
		if actual != code {
			t.Fatalf("close %d, want %d", actual, code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("missing close", code)
	}
}
func barrier(t *testing.T, r *Room) {
	t.Helper()
	done := make(chan struct{})
	r.post(func() { close(done) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("room queue blocked")
	}
}
func create(t *testing.T, d *Directory, quick bool) (protocol.Credentials, *Room) {
	t.Helper()
	c, err := d.Create(protocol.Create{Nickname: "房主", Capacity: 2, Mode: game.Speed}, quick)
	if err != nil {
		t.Fatal(err)
	}
	return c, d.Find(c.RoomID)
}

func TestSlowMailboxAndStaleDetach(t *testing.T) {
	d := NewDirectory(t.Context(), testConfig())
	defer d.Close()
	c, r := create(t, d, false)
	noReader := session.NewPeer()
	if !r.Attach(noReader, c.SessionToken, "old") {
		t.Fatal("attach")
	}
	for i := 0; i < 32; i++ {
		if !r.Submit(noReader, protocol.Command{V: 1, Type: "ping", RequestID: "heartbeat", Payload: json.RawMessage("{}")}) {
			t.Fatal("queue unexpectedly full")
		}
	}
	expectClose(t, noReader, session.Retry)
	barrier(t, r)
	healthy := session.NewPeer()
	if !r.Attach(healthy, c.SessionToken, "new") {
		t.Fatal("reattach")
	}
	expectMessage(t, healthy, "auth_ok")
	expectMessage(t, healthy, "room_state")
	r.Detach(noReader)
	r.Submit(healthy, protocol.Command{V: 1, Type: "ping", RequestID: "ping"})
	expectMessage(t, healthy, "pong")
}

func TestIdleAndExpiredSession(t *testing.T) {
	cfg := testConfig()
	cfg.OfflineTTL = 200 * time.Millisecond
	cfg.SessionTTL = 30 * time.Millisecond
	d := NewDirectory(t.Context(), cfg)
	defer d.Close()
	c, r := create(t, d, false)
	time.Sleep(40 * time.Millisecond)
	p := session.NewPeer()
	if !r.Attach(p, c.SessionToken, "hello") {
		t.Fatal("room disappeared early")
	}
	expectClose(t, p, session.InvalidSession)
	select {
	case <-r.ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("never expires")
	}
	d.Close()
	if d.Count() != 0 {
		t.Fatal("room index retained expired room")
	}
}

func TestRobotViewIgnoresOtherHandsAndDeck(t *testing.T) {
	aDeck, bDeck := card.Deck(), card.Deck()
	// The leader has the even positions through 10. Change an opponent's
	// physical card, and swap the next two cards of the hidden stock.
	bDeck[1], bDeck[12] = bDeck[12], bDeck[1]
	bDeck[13], bDeck[14] = bDeck[14], bDeck[13]
	a, err := game.New(game.Classic, 2, 0, aDeck)
	if err != nil {
		t.Fatal(err)
	}
	b, err := game.New(game.Classic, 2, 0, bDeck)
	if err != nil {
		t.Fatal(err)
	}
	r := Room{mode: game.Classic, capacity: 2, players: []*member{{bot: true}, {score: 2}}, game: a}
	viewA := r.botView(0)
	r.game = b
	viewB := r.botView(0)
	if reflect.DeepEqual(a.Hand(1), b.Hand(1)) || !reflect.DeepEqual(viewA, viewB) {
		t.Fatal("fixture or visibility differs", viewA, viewB)
	}
	if !reflect.DeepEqual(ai.Choose(context.Background(), viewA), ai.Choose(context.Background(), viewB)) {
		t.Fatal("hidden state affected AI")
	}
}

func TestOfflineCancelsRobotAndReattachSchedules(t *testing.T) {
	cfg := testConfig()
	cfg.Deal = func(mode game.Mode, n, first int) (*game.Game, error) { return game.New(mode, n, 1, card.Deck()) }
	d := NewDirectory(t.Context(), cfg)
	defer d.Close()
	d.workers <- struct{}{} // Queue the bot before it can compute.
	c, r := create(t, d, true)
	p := session.NewPeer()
	r.Attach(p, c.SessionToken, "old")
	start := expectMessage(t, p, "game_state")
	r.Detach(p)
	barrier(t, r)
	<-d.workers
	time.Sleep(470 * time.Millisecond)
	q := session.NewPeer()
	r.Attach(q, c.SessionToken, "new")
	initial := expectMessage(t, q, "game_state")
	if initial.Revision != start.Revision+2 || len(initial.Payload.(protocol.GameState).Events) != 0 {
		t.Fatalf("stale bot action committed: %#v", initial)
	}
	follow := expectMessage(t, q, "game_state")
	if follow.Revision != initial.Revision+1 {
		t.Fatal("bot failed to resume")
	}
}
