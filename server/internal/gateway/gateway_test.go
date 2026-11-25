package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/protocol"
	"github.com/duanap/gandengyan/server/internal/room"
	"github.com/duanap/gandengyan/server/internal/rules"
	"github.com/duanap/gandengyan/server/internal/session"
	"github.com/duanap/gandengyan/server/internal/webassets"
)

type packet struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	GameID    string          `json:"gameId"`
	Revision  uint64          `json:"revision"`
	Payload   json.RawMessage `json:"payload"`
}

func fixtureDeal(mode game.Mode, n, first int) (*game.Game, error) {
	if n != 2 {
		return game.New(mode, n, max(0, first), card.Deck())
	}
	prefix := []card.ID{4, 16, 8, 20, 53, 24, 52, 32, 48, 36, 12, 13, 17}
	deck := slices.Clone(prefix)
	for _, id := range card.Deck() {
		if !slices.Contains(prefix, id) {
			deck = append(deck, id)
		}
	}
	return game.New(mode, n, max(0, first), deck)
}

// Seat zero starts with a natural four-bomb or a wild five-/six-bomb in the
// larger browser test rooms. The live server never uses this deal.
func browserFixtureDeal(mode game.Mode, n, first int) (*game.Game, error) {
	if n == 2 {
		return fixtureDeal(mode, n, first)
	}
	opening := append([]card.ID{8, 9, 10, 11, 52, 53}[:n+1], []card.ID{16, 20}[:5-n]...)
	deck := card.Deck()
	for i, id := range opening {
		at := slices.Index(deck, id)
		deck[i*n], deck[at] = deck[at], deck[i*n]
	}
	return game.New(mode, n, max(0, first), deck)
}

func testGateway(t *testing.T, ttl time.Duration, deal func(game.Mode, int, int) (*game.Game, error)) (*httptest.Server, *room.Directory) {
	t.Helper()
	dir := room.NewDirectory(t.Context(), room.Config{MaxRooms: 128, OfflineTTL: ttl, SessionTTL: time.Hour, AIWorkers: 2, Deal: deal})
	s := httptest.NewServer(New(dir, Config{MaxConnections: 512, MaxPerIP: 50, TrustLoopbackProxy: true, Build: "test"}, nil))
	t.Cleanup(func() { dir.Close(); s.Close() })
	return s, dir
}
func post(t testing.TB, url string, body any) (int, []byte) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw
}
func credentials(t testing.TB, url string, req any) protocol.Credentials {
	t.Helper()
	status, body := post(t, url, req)
	if status != 200 && status != 201 {
		t.Fatalf("HTTP %d %s", status, body)
	}
	var cred protocol.Credentials
	if err := json.Unmarshal(body, &cred); err != nil {
		t.Fatal(err)
	}
	return cred
}
func dial(t testing.TB, base string, cred protocol.Credentials) *websocket.Conn {
	t.Helper()
	c, res, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(base, "http")+"/ws", http.Header{"Origin": {base}})
	if err != nil {
		if res != nil {
			res.Body.Close()
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	send(t, c, command(packet{}, "auth", "hello", protocol.Auth{RoomID: cred.RoomID, SessionToken: cred.SessionToken}))
	return c
}
func send(t testing.TB, c *websocket.Conn, cmd protocol.Command) {
	t.Helper()
	_ = c.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if err := c.WriteJSON(cmd); err != nil {
		t.Fatal(err)
	}
}
func command(state packet, kind, id string, body any) protocol.Command {
	payload, _ := json.Marshal(body)
	return protocol.Command{V: 1, Type: kind, RequestID: id, GameID: state.GameID, Revision: &state.Revision, Payload: payload}
}
func readUntil(t testing.TB, c *websocket.Conn, match func(packet) bool) packet {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var value packet
		if err := c.ReadJSON(&value); err != nil {
			t.Fatal(err)
		}
		if match(value) {
			return value
		}
	}
}
func receive(t testing.TB, c *websocket.Conn, kind string) packet {
	t.Helper()
	return readUntil(t, c, func(p packet) bool { return p.Type == kind })
}
func decode[T any](t testing.TB, data json.RawMessage) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func ack(t testing.TB, c *websocket.Conn, id, code string) packet {
	t.Helper()
	got := readUntil(t, c, func(p packet) bool { return p.RequestID == id })
	if code == "" {
		if got.Type != "action_accepted" {
			t.Fatalf("%s: %s (%s)", id, got.Type, got.Payload)
		}
	} else if got.Type != "action_rejected" || decode[protocol.Problem](t, got.Payload).Code != code {
		t.Fatalf("%s: expected %s, received %s (%s)", id, code, got.Type, got.Payload)
	}
	return got
}
func closeCode(t *testing.T, c *websocket.Conn, code int) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			if !websocket.IsCloseError(err, code) {
				t.Fatal(err)
			}
			return
		}
	}
}
func waitingPair(t *testing.T, s *httptest.Server, mode game.Mode) (protocol.Credentials, protocol.Credentials, *websocket.Conn, *websocket.Conn, packet, packet) {
	t.Helper()
	ca := credentials(t, s.URL+"/api/rooms", protocol.Create{Nickname: "甲", Capacity: 2, Mode: mode})
	cb := credentials(t, s.URL+"/api/rooms/join", protocol.Join{RoomID: ca.RoomID, Nickname: "乙"})
	a, b := dial(t, s.URL, ca), dial(t, s.URL, cb)
	first := func(p packet) bool {
		return p.Type == "room_state" && allOnline(decode[protocol.RoomState](t, p.Payload))
	}
	return ca, cb, a, b, readUntil(t, a, first), readUntil(t, b, first)
}
func allOnline(s protocol.RoomState) bool {
	if len(s.Players) != s.Capacity {
		return false
	}
	for _, p := range s.Players {
		if !p.Online {
			return false
		}
	}
	return true
}

func TestProtocolAuthorityAndIsolation(t *testing.T) {
	s, _ := testGateway(t, time.Minute, fixtureDeal)
	ca, cb, a, b, wa, wb := waitingPair(t, s, game.Classic)
	empty := struct{}{}
	send(t, b, command(wb, "start_game", "non-host", empty))
	ack(t, b, "non-host", "NOT_HOST")
	start := command(wa, "start_game", "start", empty)
	send(t, a, start)
	started := ack(t, a, "start", "")
	ga, gb := receive(t, a, "game_state"), receive(t, b, "game_state")
	as, bs := decode[protocol.GameState](t, ga.Payload), decode[protocol.GameState](t, gb.Payload)
	if as.CurrentID != ca.PlayerID || len(as.SelfHand) != 6 || len(bs.SelfHand) != 5 || len(as.Notices) != 1 || len(bs.Notices) != 0 {
		t.Fatal("N-08/G-13: projections invalid")
	}
	for _, id := range bs.SelfHand {
		if slices.Contains(as.SelfHand, id) {
			t.Fatal("overlap")
		}
	}
	if strings.Contains(string(gb.Payload), as.Notices[0].ID) || strings.Contains(string(gb.Payload), "sessionToken") || strings.Contains(string(gb.Payload), `"deck":`) {
		t.Fatal("private fields leaked")
	}
	send(t, a, start)
	if replay := ack(t, a, "start", ""); replay.Revision != started.Revision || replay.GameID != started.GameID {
		t.Fatal("original ack must be replayed")
	}
	receive(t, a, "game_state")
	send(t, a, command(ga, "start_game", "start-again", empty))
	ack(t, a, "start-again", "INVALID_PHASE")

	invalid := command(ga, "play_cards", "bad-card", json.RawMessage(`{"cardIds":[null]}`))
	send(t, a, invalid)
	ack(t, a, "bad-card", "INVALID_INPUT")
	send(t, a, command(ga, "play_cards", "range", json.RawMessage(`{"cardIds":[54]}`)))
	ack(t, a, "range", "INVALID_CARD_IDS")
	send(t, a, command(ga, "play_cards", "other-card", json.RawMessage(`{"cardIds":[16]}`)))
	ack(t, a, "other-card", "INVALID_CARD_IDS")
	send(t, a, command(ga, "play_cards", "forged-owner", json.RawMessage(`{"playerId":"`+cb.PlayerID+`","cardIds":[4]}`)))
	ack(t, a, "forged-owner", "INVALID_INPUT")
	send(t, a, command(ga, "preview_play", "bad-rest", json.RawMessage(`{"cardIds":[4,8,12,53]}`)))
	preview := receive(t, a, "play_preview")
	if choice := decode[protocol.Preview](t, preview.Payload); choice.Problem == nil || choice.Problem.Code != "INVALID_REMAINING_SPECIALS" || preview.Revision != ga.Revision {
		t.Fatal("preview accepted an illegal selection", choice)
	}

	send(t, b, command(gb, "play_cards", "other-turn", rules.Request{CardIDs: []card.ID{16}}))
	ack(t, b, "other-turn", "NOT_YOUR_TURN")
	choose := rules.Request{CardIDs: []card.ID{4, 8, 53}, Pattern: &rules.Declaration{Shape: rules.Shape{Type: rules.Straight, Rank: 4, Length: 3}}}
	play := command(ga, "play_cards", "play", choose)
	send(t, a, play)
	accepted := ack(t, a, "play", "")
	followA, followB := receive(t, a, "game_state"), receive(t, b, "game_state")
	if accepted.Revision != ga.Revision+1 || followA.Revision != followB.Revision {
		t.Fatal("atomic revision")
	}
	if got := decode[protocol.GameState](t, followB.Payload); got.LastPattern == nil || got.LastPattern.Rank != 4 || !slices.Equal(got.SelfHand, bs.SelfHand) {
		t.Fatal("wrong public play or private hand", got)
	}
	send(t, a, play)
	ack(t, a, "play", "")
	replay := receive(t, a, "game_state")
	if replay.Revision != followA.Revision {
		t.Fatal("N-04 duplicate mutated")
	}
	play.Payload = json.RawMessage(`{"cardIds":[12]}`)
	send(t, a, play)
	ack(t, a, "play", "DUPLICATE_REQUEST_CONFLICT")

	stale := command(gb, "pass", "stale", empty)
	send(t, b, stale)
	ack(t, b, "stale", "STALE_STATE")
	receive(t, b, "game_state")
	fakeGame := command(followB, "pass", "wrong-game", empty)
	fakeGame.GameID = "unrelated"
	send(t, b, fakeGame)
	ack(t, b, "wrong-game", "STALE_STATE")
	receive(t, b, "game_state")

	send(t, b, command(followB, "pass", "pass", empty))
	ack(t, b, "pass", "")
	drawA, drawB := receive(t, a, "game_state"), receive(t, b, "game_state")
	for i, p := range []packet{drawA, drawB} {
		state := decode[protocol.GameState](t, p.Payload)
		if state.Phase != game.Leading || state.DeckCount != 41 || state.Round != 2 || !slices.Contains(state.SelfHand, []card.ID{13, 17}[i]) ||
			slices.Contains(state.SelfHand, []card.ID{17, 13}[i]) {
			t.Fatal("N-09 draw snapshot", state)
		}
		for _, e := range state.Events {
			if e.Type == "cards_drawn" && (e.Pattern != nil || e.Count != 1) {
				t.Fatal("public draw leaked card", e)
			}
		}
	}

	a2 := dial(t, s.URL, ca)
	receive(t, a2, "auth_ok")
	restored := receive(t, a2, "game_state")
	closeCode(t, a, session.Replaced)
	rs := decode[protocol.GameState](t, restored.Payload)
	if !slices.Equal(rs.SelfHand, decode[protocol.GameState](t, drawA.Payload).SelfHand) || len(rs.Notices) != 1 || rs.Notices[0].ID != as.Notices[0].ID {
		t.Fatal("N-10/11, G-14: restore mismatch")
	}
	send(t, a2, command(restored, "ping", "still-here", empty))
	receive(t, a2, "pong")
	other := credentials(t, s.URL+"/api/rooms", protocol.Create{Nickname: "他桌", Capacity: 2, Mode: game.Speed})
	ca.RoomID = other.RoomID
	closeCode(t, dial(t, s.URL, ca), session.InvalidSession)
}

func TestLobbyHostTransferAndLeave(t *testing.T) {
	s, directory := testGateway(t, time.Minute, nil)
	ca, cb, a, b, wa, _ := waitingPair(t, s, game.Speed)
	send(t, a, command(wa, "leave_room", "leave-a", struct{}{}))
	ack(t, a, "leave-a", "")
	closeCode(t, a, session.Left)
	after := receive(t, b, "room_state")
	state := decode[protocol.RoomState](t, after.Payload)
	if state.HostID != cb.PlayerID || len(state.Players) != 1 {
		t.Fatal("wrong host or remaining seats")
	}
	send(t, b, command(after, "leave_room", "leave-b", struct{}{}))
	ack(t, b, "leave-b", "")
	closeCode(t, b, session.Left)
	closeCode(t, dial(t, s.URL, ca), session.InvalidSession)
	deadline := time.Now().Add(time.Second)
	for directory.Count() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if directory.Count() != 0 {
		t.Fatal("vacated room not reclaimed")
	}
}

func TestConcurrentCommandsAndReplay(t *testing.T) {
	s, _ := testGateway(t, time.Minute, fixtureDeal)
	_, _, a, b, wa, _ := waitingPair(t, s, game.Speed)
	empty := struct{}{}
	send(t, a, command(wa, "start_game", "start", empty))
	ack(t, a, "start", "")
	ga, gb := receive(t, a, "game_state"), receive(t, b, "game_state")
	// Different connections race for the same revision. Only the active seat may commit.
	done := make(chan error, 1)
	go func() { done <- b.WriteJSON(command(gb, "pass", "wrong-seat", empty)) }()
	send(t, a, command(ga, "play_cards", "legal", rules.Request{CardIDs: []card.ID{48}}))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ack(t, a, "legal", "")
	state := receive(t, a, "game_state")
	rejected := readUntil(t, b, func(p packet) bool { return p.RequestID == "wrong-seat" })
	problem := decode[protocol.Problem](t, rejected.Payload)
	if rejected.Type != "action_rejected" || problem.Code != "NOT_YOUR_TURN" && problem.Code != "STALE_STATE" || state.Revision != ga.Revision+1 {
		t.Fatal("N-07 simultaneous commands", problem)
	}
}

func TestMatrixRooms(t *testing.T) {
	for n := 2; n <= 5; n++ {
		for _, mode := range []game.Mode{game.Classic, game.Speed} {
			t.Run(fmt.Sprintf("%d/%s", n, mode), func(t *testing.T) {
				t.Parallel()
				// Separate loopback servers keep admission limits independent.
				local, _ := testGateway(t, time.Minute, nil)
				owner := credentials(t, local.URL+"/api/rooms", protocol.Create{Nickname: "房主", Capacity: n, Mode: mode})
				peers := []*websocket.Conn{dial(t, local.URL, owner)}
				for i := 1; i < n; i++ {
					c := credentials(t, local.URL+"/api/rooms/join", protocol.Join{RoomID: owner.RoomID, Nickname: fmt.Sprint(i)})
					peers = append(peers, dial(t, local.URL, c))
				}
				w := readUntil(t, peers[0], func(p packet) bool {
					return p.Type == "room_state" && allOnline(decode[protocol.RoomState](t, p.Payload))
				})
				send(t, peers[0], command(w, "start_game", "start", struct{}{}))
				ack(t, peers[0], "start", "")
				hands := map[card.ID]bool{}
				for _, peer := range peers {
					state := decode[protocol.GameState](t, receive(t, peer, "game_state").Payload)
					if state.RoomID != owner.RoomID || state.Capacity != n || state.DeckCount != 53-5*n || state.Mode != mode {
						t.Fatal("N-01", state)
					}
					for _, id := range state.SelfHand {
						if hands[id] {
							t.Fatal("duplicate card")
						}
						hands[id] = true
					}
				}
				if len(hands) != 5*n+1 {
					t.Fatal("bad deal")
				}
			})
		}
	}
}

func TestGatewayInputLimits(t *testing.T) {
	s, _ := testGateway(t, time.Minute, nil)
	_, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http")+"/ws", http.Header{"Origin": {"https://example.invalid"}})
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatal("N-16 origin check", err)
	}
	response.Body.Close()
	cred := credentials(t, s.URL+"/api/rooms", protocol.Create{Nickname: "房主", Capacity: 2, Mode: game.Speed})
	c := dial(t, s.URL, cred)
	receive(t, c, "room_state")
	if err := c.WriteMessage(websocket.TextMessage, bytes.Repeat([]byte{'x'}, maxFrame+1)); err != nil {
		t.Fatal(err)
	}
	closeCode(t, c, websocket.CloseMessageTooBig)
	c = dial(t, s.URL, cred)
	receive(t, c, "room_state")
	if err := c.WriteMessage(websocket.TextMessage, []byte("{")); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("invalid JSON left connection open")
	}
	found := false
	for i := 0; i < 40; i++ {
		status, _ := post(t, s.URL+"/api/rooms/join", protocol.Join{Nickname: "甲", RoomID: "013579"})
		if status == 429 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("N-16 room admission not throttled")
	}
}

func TestBrowserFixture(t *testing.T) {
	addr := os.Getenv("GDY_E2E_FIXTURE_ADDR")
	if addr == "" {
		return
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" {
		t.Fatal("fixture must listen on IPv4 loopback")
	}
	assets, ok := webassets.Files()
	if !ok {
		t.Fatal("run the embedded build before E2E")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dir := room.NewDirectory(ctx, room.Config{MaxRooms: 64, OfflineTTL: time.Minute, SessionTTL: time.Hour, AIWorkers: 2, Deal: browserFixtureDeal})
	defer dir.Close()
	handler := New(dir, Config{MaxConnections: 100, MaxPerIP: 30, Build: "e2e-fixture"}, assets)
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		t.Fatal(err)
	}
}
