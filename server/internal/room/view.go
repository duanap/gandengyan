package room

import (
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/protocol"
	"github.com/duanap/gandengyan/server/internal/rules"
)

func (r *Room) playerID(seat int) string {
	if seat < 0 {
		return ""
	}
	return r.players[seat].id
}

func (r *Room) snapshot(self *member, events []protocol.Event) protocol.Message {
	roomState := protocol.RoomState{RoomID: r.id, Mode: r.mode, Capacity: r.capacity, Phase: game.Waiting, HostID: r.host, SelfID: self.id, Allowed: []string{}}
	for _, p := range r.players {
		if p == nil {
			continue
		}
		view := protocol.Player{ID: p.id, Seat: p.seat, Nickname: p.nickname, Online: p.online(), Bot: p.bot, Score: p.score}
		if r.game != nil {
			view.HandCount = r.game.HandCount(p.seat)
		}
		roomState.Players = append(roomState.Players, view)
	}
	if r.game == nil {
		roomState.Allowed = append(roomState.Allowed, "leave_room")
		if r.host == self.id {
			if r.ready() {
				roomState.Allowed = append(roomState.Allowed, "start_game")
			}
			if r.freeSeat() >= 0 {
				roomState.Allowed = append(roomState.Allowed, "add_bot")
			}
			roomState.Allowed = append(roomState.Allowed, "remove_bot")
		}
		return r.message("room_state", "", roomState)
	}
	g := r.game
	roomState.Phase = g.Phase()
	state := protocol.GameState{
		RoomState: roomState, CurrentID: r.playerID(g.Current()), LeaderID: r.playerID(g.Leader()),
		LastID: r.playerID(g.Last()), Round: g.Round(), PassCount: g.PassCount(), DeckCount: g.DeckCount(),
		SelfHand: g.Hand(self.seat), LastPattern: g.LastPattern(), Notices: []protocol.Notice{}, Events: events,
	}
	if g.Notice(self.seat) {
		state.Notices = append(state.Notices, protocol.Notice{
			ID: r.gameID + "-" + self.id + "-double-joker", Code: "DOUBLE_JOKER",
			Message: "你同时持有大小王。请搭配普通牌使用，双王不能单独打出。",
		})
	}
	if ctx, err := g.Context(self.seat); err == nil {
		state.Allowed = append(state.Allowed, "play_cards", "preview_play", "hint")
		state.CanPlay = len(rules.GenerateLegalMoves(ctx)) > 0
		if g.Phase() == game.Following {
			state.Allowed = append(state.Allowed, "pass")
		}
	}
	if g.Phase() == game.Finished {
		state.Allowed = append(state.Allowed, "leave_room")
		if r.host == self.id && r.ready() {
			state.Allowed = append(state.Allowed, "restart_game")
		}
		state.WinnerID = r.lastWinner
		for _, s := range g.Settlement() {
			state.Settlement = append(state.Settlement, protocol.Result{PlayerID: r.playerID(s.Seat), HandCount: s.HandCount, Delta: s.Delta})
		}
	}
	return r.message("game_state", "", state)
}
