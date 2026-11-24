package room

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/protocol"
	"github.com/duanap/gandengyan/server/internal/rules"
	"github.com/duanap/gandengyan/server/internal/session"
)

const (
	errInput     rules.Error = "INVALID_INPUT"
	errBusy      rules.Error = "SERVER_BUSY"
	errMissing   rules.Error = "ROOM_NOT_FOUND"
	errPhase     rules.Error = "INVALID_PHASE"
	errFull      rules.Error = "ROOM_FULL"
	errAuth      rules.Error = "UNAUTHORIZED"
	errStale     rules.Error = "STALE_STATE"
	errHost      rules.Error = "NOT_HOST"
	errReady     rules.Error = "NOT_READY"
	errDuplicate rules.Error = "DUPLICATE_REQUEST_CONFLICT"
)

type member struct {
	id       string
	nickname string
	bot      bool
	seat     int
	score    int
	peer     *session.Peer
	digest   [32]byte
	expires  time.Time
	requests map[requestKey]remembered
	order    []requestKey
}

func (p *member) online() bool { return p.bot || p.peer != nil }

type requestKey struct{ gameID, id string }
type remembered struct {
	hash     [32]byte
	gameID   string
	revision uint64
	code     string
	when     time.Time
}

type Room struct {
	id           string
	mode         game.Mode
	capacity     int
	players      []*member
	host         string
	game         *game.Game
	gameID       string
	lastWinner   string
	revision     uint64
	queue        chan func()
	ctx          context.Context
	cancel       context.CancelFunc
	dir          *Directory
	offlineSince time.Time
	quick        bool

	aiCancel   context.CancelFunc
	aiRevision uint64
}

func (r *Room) run() {
	ticker := time.NewTicker(min(r.dir.cfg.OfflineTTL/2, time.Minute))
	defer ticker.Stop()
	defer r.dir.remove(r)
	defer r.cancel()
	defer r.cancelAI()
	defer func() {
		for _, p := range r.players {
			if p != nil && p.peer != nil {
				p.peer.Close(session.Retry)
			}
		}
	}()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			if !r.offlineSince.IsZero() && time.Since(r.offlineSince) >= r.dir.cfg.OfflineTTL {
				return
			}
		case command := <-r.queue:
			command()
			r.scheduleAI()
		}
	}
}

func (r *Room) try(command func()) bool {
	if r.ctx.Err() != nil {
		return false
	}
	select {
	case r.queue <- command:
		return true
	default:
		return false
	}
}
func (r *Room) post(command func()) {
	select {
	case r.queue <- command:
	case <-r.ctx.Done():
	}
}

func (r *Room) freeSeat() int {
	for i, p := range r.players {
		if p == nil {
			return i
		}
	}
	return -1
}
func (r *Room) addHuman(nickname string) (protocol.Credentials, error) {
	seat := r.freeSeat()
	if seat < 0 {
		return protocol.Credentials{}, errFull
	}
	id, err := session.Token(12)
	if err != nil {
		return protocol.Credentials{}, errBusy
	}
	token, err := session.Token(32)
	if err != nil {
		return protocol.Credentials{}, errBusy
	}
	r.players[seat] = &member{id: id, nickname: nickname, seat: seat, digest: session.Digest(token), expires: time.Now().Add(r.dir.cfg.SessionTTL), requests: make(map[requestKey]remembered)}
	if r.host == "" {
		r.host = id
	}
	if !r.offlineSince.IsZero() {
		r.offlineSince = time.Now()
	}
	r.revision++
	return protocol.Credentials{RoomID: r.id, PlayerID: id, SessionToken: token, ProtocolVersion: protocol.Version}, nil
}
func (r *Room) addBot() error {
	seat := r.freeSeat()
	if seat < 0 {
		return errFull
	}
	id, err := session.Token(12)
	if err != nil {
		return errBusy
	}
	r.players[seat] = &member{id: id, nickname: fmt.Sprintf("牌友 %d", seat+1), seat: seat, bot: true}
	return nil
}
func (r *Room) ready() bool {
	for _, p := range r.players {
		if p == nil || !p.online() {
			return false
		}
	}
	return true
}
func (r *Room) updateIdle() {
	for _, p := range r.players {
		if p != nil && p.peer != nil {
			r.offlineSince = time.Time{}
			return
		}
	}
	if r.offlineSince.IsZero() {
		r.offlineSince = time.Now()
	}
}
func (r *Room) byPeer(peer *session.Peer) *member {
	for _, p := range r.players {
		if p != nil && p.peer == peer {
			return p
		}
	}
	return nil
}

func (r *Room) Attach(peer *session.Peer, token, requestID string) bool {
	return r.try(func() {
		digest := session.Digest(token)
		for _, p := range r.players {
			if p == nil || p.bot || p.digest != digest || !time.Now().Before(p.expires) {
				continue
			}
			wasOnline := p.online()
			if wasOnline {
				p.peer.Close(session.Replaced)
			}
			p.peer = peer
			p.expires = time.Now().Add(r.dir.cfg.SessionTTL)
			r.updateIdle()
			if !wasOnline {
				r.revision++
			}
			r.send(peer, r.message("auth_ok", requestID, struct {
				ID string `json:"playerId"`
			}{p.id}))
			if r.quick && r.game == nil {
				if err := r.start(); err == nil {
					r.revision++
					r.broadcast([]protocol.Event{{Type: "game_started"}})
					return
				}
			}
			if wasOnline {
				r.send(peer, r.snapshot(p, nil))
			} else {
				r.broadcast(nil)
			}
			return
		}
		peer.Close(session.InvalidSession)
	})
}

func (r *Room) Detach(peer *session.Peer) {
	r.post(func() {
		p := r.byPeer(peer)
		if p == nil {
			return
		}
		p.peer = nil
		r.updateIdle()
		r.revision++
		r.broadcast(nil)
	})
}

func (r *Room) Submit(peer *session.Peer, c protocol.Command) bool {
	return r.try(func() {
		p := r.byPeer(peer)
		if p == nil {
			peer.Close(session.Replaced)
			return
		}
		r.command(p, c)
	})
}

func (r *Room) message(kind, id string, body any) protocol.Message {
	return protocol.Message{V: protocol.Version, Type: kind, RequestID: id, GameID: r.gameID, Revision: r.revision, Payload: body}
}

func (r *Room) send(peer *session.Peer, message protocol.Message) bool {
	select {
	case peer.Out <- message:
		return true
	default:
		peer.Close(session.Retry)
		return false
	}
}

func (r *Room) broadcast(events []protocol.Event) {
	for i := range events {
		events[i].Index = i
	}
	for _, p := range r.players {
		if p != nil && p.peer != nil {
			r.send(p.peer, r.snapshot(p, events))
		}
	}
}

func (r *Room) start() error {
	if !r.ready() {
		return errReady
	}
	id, err := session.Token(12)
	if err != nil {
		return errBusy
	}
	first := -1
	for i, p := range r.players {
		if p.id == r.lastWinner {
			first = i
			break
		}
	}
	next, err := r.dir.cfg.Deal(r.mode, r.capacity, first)
	if err != nil {
		return errBusy
	}
	r.game, r.gameID = next, id
	return nil
}

func secureDeal(mode game.Mode, capacity, first int) (*game.Game, error) {
	deck, err := card.Shuffle(card.SecureIndex)
	if err != nil {
		return nil, err
	}
	if first < 0 {
		first, err = card.SecureIndex(capacity)
		if err != nil {
			return nil, err
		}
	}
	return game.New(mode, capacity, first, deck)
}

func (r *Room) leave(p *member) {
	r.players[p.seat] = nil
	// Keep scores and the previous winner even when the roster changes.
	r.game, r.gameID = nil, ""
	r.quick = false
	if r.host == p.id {
		r.host = ""
		for _, other := range r.players {
			if other != nil && !other.bot {
				r.host = other.id
				break
			}
		}
	}
	if r.host == "" {
		r.cancel()
	}
	r.updateIdle()
}

func (r *Room) validateVersion(c protocol.Command) error {
	if c.Revision == nil || *c.Revision != r.revision || c.GameID != r.gameID {
		return errStale
	}
	return nil
}
func (r *Room) requestHash(c protocol.Command) [32]byte {
	var normalized any
	d := json.NewDecoder(bytes.NewReader(c.Payload))
	d.UseNumber()
	if d.Decode(&normalized) == nil && d.Decode(new(any)) == io.EOF {
		c.Payload, _ = json.Marshal(normalized)
	}
	data, _ := json.Marshal(c)
	return sha256.Sum256(data)
}

func (p *member) remember(key requestKey, hash [32]byte, gameID string, revision uint64, err error) {
	now := time.Now()
	for len(p.order) > 0 && (len(p.order) >= 256 || now.Sub(p.requests[p.order[0]].when) > 10*time.Minute) {
		delete(p.requests, p.order[0])
		p.order = p.order[1:]
	}
	code := ""
	if err != nil {
		code = err.Error()
	}
	p.requests[key] = remembered{hash, gameID, revision, code, now}
	p.order = append(p.order, key)
}
func (r *Room) ack(peer *session.Peer, c protocol.Command, err error, revision uint64) {
	r.ackVersion(peer, c, err, r.gameID, revision)
}
func (r *Room) ackVersion(peer *session.Peer, c protocol.Command, err error, gameID string, revision uint64) {
	message := r.message("action_accepted", c.RequestID, struct{}{})
	message.Revision = revision
	message.GameID = gameID
	if err != nil {
		message.Type = "action_rejected"
		message.Payload = protocol.Issue(err.Error())
	}
	r.send(peer, message)
}

func (r *Room) command(p *member, c protocol.Command) {
	peer := p.peer
	if c.Type == "ping" {
		r.send(peer, r.message("pong", c.RequestID, struct{}{}))
		return
	}
	if c.Type == "preview_play" || c.Type == "hint" {
		r.query(p, c)
		return
	}
	key := requestKey{c.GameID, c.RequestID}
	hash := r.requestHash(c)
	if saved, ok := p.requests[key]; ok && time.Since(saved.when) <= 10*time.Minute {
		if saved.hash != hash {
			r.ack(peer, c, errDuplicate, r.revision)
			return
		}
		var err error
		if saved.code != "" {
			err = rules.Error(saved.code)
		}
		r.ackVersion(peer, c, err, saved.gameID, saved.revision)
		r.send(peer, r.snapshot(p, nil))
		return
	}
	err := r.validateVersion(c)
	var events []protocol.Event
	var disconnect bool
	if err == nil {
		events, disconnect, err = r.execute(p, c)
	}
	if err != nil {
		p.remember(key, hash, r.gameID, r.revision, err)
		r.ack(peer, c, err, r.revision)
		if err == errStale {
			r.send(peer, r.snapshot(p, nil))
		}
		return
	}
	r.revision++
	p.remember(key, hash, r.gameID, r.revision, nil)
	if disconnect {
		sent := r.message("action_accepted", c.RequestID, struct{}{})
		sent.CloseAfter = session.Left
		r.send(peer, sent)
	} else {
		r.ack(peer, c, nil, r.revision)
	}
	r.broadcast(events)
}

func (r *Room) query(p *member, c protocol.Command) {
	err := r.validateVersion(c)
	var ctx rules.Context
	if err == nil {
		if r.game == nil {
			err = errPhase
		} else {
			ctx, err = r.game.Context(p.seat)
		}
	}
	if err != nil {
		r.ack(p.peer, c, err, r.revision)
		if err == errStale {
			r.send(p.peer, r.snapshot(p, nil))
		}
		return
	}
	if c.Type == "hint" {
		if c.Parse(new(struct{})) != nil {
			r.ack(p.peer, c, errInput, r.revision)
			return
		}
		moves := rules.GenerateLegalMoves(ctx)
		var best *rules.Pattern
		for _, m := range moves {
			if best == nil || m.Length > best.Length || m.Length == best.Length && len(m.Wildcards) < len(best.Wildcards) {
				v := m
				best = &v
			}
		}
		r.send(p.peer, r.message("play_hint", c.RequestID, best))
		return
	}
	var req struct {
		CardIDs []card.ID `json:"cardIds"`
	}
	if c.Parse(&req) != nil {
		r.ack(p.peer, c, errInput, r.revision)
		return
	}
	options, err := rules.Preview(ctx, req.CardIDs)
	response := protocol.Preview{CardIDs: req.CardIDs, Options: options}
	if err != nil {
		p := protocol.Issue(err.Error())
		response.Problem = &p
		response.Options = []rules.Pattern{}
	}
	r.send(p.peer, r.message("play_preview", c.RequestID, response))
}

func (r *Room) execute(p *member, c protocol.Command) ([]protocol.Event, bool, error) {
	if c.Type == "play_cards" || c.Type == "pass" {
		events, err := r.gameAction(p, c)
		return events, false, err
	}
	switch c.Type {
	case "start_game", "restart_game", "add_bot", "remove_bot":
		if p.id != r.host {
			return nil, false, errHost
		}
	}
	if c.Type != "remove_bot" && c.Parse(new(struct{})) != nil {
		return nil, false, errInput
	}
	switch c.Type {
	case "start_game", "restart_game":
		if c.Type == "start_game" && r.game != nil || c.Type == "restart_game" && (r.game == nil || r.game.Phase() != game.Finished) {
			return nil, false, errPhase
		}
		if err := r.start(); err != nil {
			return nil, false, err
		}
		return []protocol.Event{{Type: "game_started"}}, false, nil
	case "add_bot":
		if r.game != nil {
			return nil, false, errPhase
		}
		return nil, false, r.addBot()
	case "remove_bot":
		if r.game != nil {
			return nil, false, errPhase
		}
		var req struct {
			ID string `json:"playerId"`
		}
		if c.Parse(&req) != nil {
			return nil, false, errInput
		}
		for i, candidate := range r.players {
			if candidate != nil && candidate.bot && candidate.id == req.ID {
				r.players[i] = nil
				return nil, false, nil
			}
		}
		return nil, false, errInput
	case "leave_room":
		if r.game != nil && r.game.Phase() != game.Finished {
			p.peer = nil
			r.updateIdle()
		} else {
			r.leave(p)
		}
		return nil, true, nil
	default:
		return nil, false, errInput
	}
}

func (r *Room) gameAction(p *member, c protocol.Command) ([]protocol.Event, error) {
	if r.game == nil {
		return nil, errPhase
	}
	if c.Type == "pass" {
		if c.Parse(new(struct{})) != nil {
			return nil, errInput
		}
		return r.applyPass(p)
	}
	var req rules.Request
	if c.Parse(&req) != nil {
		return nil, errInput
	}
	return r.applyPlay(p, req)
}

func (r *Room) applyPass(p *member) ([]protocol.Event, error) {
	draws, err := r.game.Pass(p.seat)
	if err != nil {
		return nil, err
	}
	events := []protocol.Event{{Type: "player_passed", PlayerID: p.id}}
	for _, draw := range draws {
		events = append(events, protocol.Event{Type: "cards_drawn", PlayerID: r.players[draw.Seat].id, Count: 1})
	}
	return events, nil
}
func (r *Room) applyPlay(p *member, req rules.Request) ([]protocol.Event, error) {
	pattern, err := r.game.Play(p.seat, req)
	if err != nil {
		return nil, err
	}
	events := []protocol.Event{{Type: "cards_played", PlayerID: p.id, Pattern: &pattern}}
	if r.game.Phase() == game.Finished {
		r.lastWinner = p.id
		for _, s := range r.game.Settlement() {
			r.players[s.Seat].score += s.Delta
		}
		events = append(events, protocol.Event{Type: "game_finished", PlayerID: p.id})
	}
	return events, nil
}
