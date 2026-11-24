package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/rules"
)

const Version = 1

type Command struct {
	V         int             `json:"v"`
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	GameID    string          `json:"gameId,omitempty"`
	Revision  *uint64         `json:"revision,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

type Message struct {
	V          int    `json:"v"`
	Type       string `json:"type"`
	RequestID  string `json:"requestId,omitempty"`
	GameID     string `json:"gameId,omitempty"`
	Revision   uint64 `json:"revision"`
	Payload    any    `json:"payload"`
	CloseAfter int    `json:"-"`
}

type Credentials struct {
	RoomID          string `json:"roomId"`
	PlayerID        string `json:"playerId"`
	SessionToken    string `json:"sessionToken"`
	ProtocolVersion int    `json:"protocolVersion"`
}
type Auth struct {
	RoomID       string `json:"roomId"`
	SessionToken string `json:"sessionToken"`
}
type Create struct {
	Nickname string    `json:"nickname"`
	Capacity int       `json:"capacity"`
	Mode     game.Mode `json:"mode"`
}
type Join struct {
	RoomID   string `json:"roomId"`
	Nickname string `json:"nickname"`
}
type Player struct {
	ID        string `json:"playerId"`
	Seat      int    `json:"seat"`
	Nickname  string `json:"nickname"`
	Online    bool   `json:"online"`
	Bot       bool   `json:"bot"`
	HandCount int    `json:"handCount"`
	Score     int    `json:"score"`
}
type RoomState struct {
	RoomID   string     `json:"roomId"`
	Mode     game.Mode  `json:"mode"`
	Capacity int        `json:"capacity"`
	Phase    game.Phase `json:"phase"`
	HostID   string     `json:"hostPlayerId"`
	SelfID   string     `json:"selfPlayerId"`
	Players  []Player   `json:"players"`
	Allowed  []string   `json:"allowedActions"`
}
type GameState struct {
	RoomState
	CurrentID   string         `json:"currentPlayerId"`
	LeaderID    string         `json:"leaderPlayerId"`
	LastID      string         `json:"lastPlayPlayerId"`
	Round       int            `json:"roundNumber"`
	PassCount   int            `json:"passCount"`
	DeckCount   int            `json:"deckCount"`
	SelfHand    []card.ID      `json:"selfHand"`
	LastPattern *rules.Pattern `json:"lastPattern"`
	CanPlay     bool           `json:"canPlay"`
	Notices     []Notice       `json:"privateNotices"`
	WinnerID    string         `json:"winnerId,omitempty"`
	Settlement  []Result       `json:"settlement,omitempty"`
	Events      []Event        `json:"events,omitempty"`
}
type Result struct {
	PlayerID  string `json:"playerId"`
	HandCount int    `json:"handCount"`
	Delta     int    `json:"delta"`
}
type Notice struct {
	ID      string `json:"noticeId"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Event struct {
	Type     string         `json:"type"`
	Index    int            `json:"eventIndex"`
	PlayerID string         `json:"playerId,omitempty"`
	Count    int            `json:"count,omitempty"`
	Pattern  *rules.Pattern `json:"pattern,omitempty"`
}
type Preview struct {
	CardIDs []card.ID       `json:"cardIds"`
	Options []rules.Pattern `json:"options"`
	Problem *Problem        `json:"problem,omitempty"`
}
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var messages = map[string]string{
	"UNAUTHORIZED":               "会话已失效，无法恢复此席位。",
	"ROOM_NOT_FOUND":             "房间不存在或已回收。",
	"ROOM_FULL":                  "房间已满。",
	"INVALID_INPUT":              "请检查输入内容。",
	"INVALID_PHASE":              "当前阶段不能执行此操作。",
	"NOT_YOUR_TURN":              "还没轮到你。",
	"NOT_HOST":                   "只有房主能执行此操作。",
	"NOT_READY":                  "需要坐满所有席位且所有玩家在线。",
	"INVALID_CARD_IDS":           "所选手牌有误，请重新选择。",
	"INVALID_PATTERN":            "这组牌无法组成指定牌型。",
	"PATTERN_CHOICE_REQUIRED":    "请选择万能牌的用法。",
	"CANNOT_BEAT":                "需要同牌型、同张数且恰好大一级，也可尝试用 2 或炸弹。",
	"INVALID_REMAINING_SPECIALS": "不能仅剩单张 2、对 2、王或 2 加王，请调整组合。",
	"INVALID_FINAL_PLAY":         "单张 2 或对 2 不能作为最后一手。",
	"LEADING_CANNOT_PASS":        "领出时必须出牌。",
	"GAME_FINISHED":              "本局已结算。",
	"STALE_STATE":                "牌桌已同步，请重新确认操作。",
	"DUPLICATE_REQUEST_CONFLICT": "请求编号已被其他操作使用。",
	"RATE_LIMITED":               "操作过于频繁，请稍后再试。",
	"SERVER_BUSY":                "服务繁忙，请稍后重试。",
	"UNSUPPORTED_VERSION":        "请刷新页面以更新游戏版本。",
}

func Issue(code string) Problem { return Problem{code, messages[code]} }

func Decode(data []byte, target any) error { return Read(bytes.NewReader(data), target) }
func Read(r io.Reader, target any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("expected one object")
	}
	return nil
}

func (c Command) Parse(target any) error {
	if len(c.Payload) == 0 || c.Payload[0] != '{' {
		return errors.New("payload must be an object")
	}
	return Decode(c.Payload, target)
}
