package room

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/duanap/gandengyan/server/internal/card"
	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/protocol"
)

type Config struct {
	MaxRooms   int
	OfflineTTL time.Duration
	SessionTTL time.Duration
	AIWorkers  int
	Deal       func(game.Mode, int, int) (*game.Game, error)
}
type Directory struct {
	ctx     context.Context
	cancel  context.CancelFunc
	cfg     Config
	mu      sync.RWMutex
	rooms   map[string]*Room
	workers chan struct{}
	wg      sync.WaitGroup
	aiWG    sync.WaitGroup
}

func NewDirectory(parent context.Context, cfg Config) *Directory {
	if cfg.Deal == nil {
		cfg.Deal = secureDeal
	}
	ctx, cancel := context.WithCancel(parent)
	return &Directory{ctx: ctx, cancel: cancel, cfg: cfg, rooms: make(map[string]*Room), workers: make(chan struct{}, cfg.AIWorkers)}
}

func (d *Directory) Close() {
	d.mu.Lock()
	d.cancel()
	d.mu.Unlock()
	d.wg.Wait()
	d.aiWG.Wait()
}
func (d *Directory) Find(id string) *Room {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.rooms[id]
}
func (d *Directory) Count() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.rooms)
}
func (d *Directory) remove(r *Room) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rooms[r.id] == r {
		delete(d.rooms, r.id)
	}
}

var roomID = regexp.MustCompile("^[0-9]{6}$")

func nickname(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if utf8.RuneCountInString(s) < 1 || utf8.RuneCountInString(s) > 16 || strings.ContainsFunc(s, unicode.IsControl) {
		return "", errInput
	}
	return s, nil
}

func (d *Directory) Create(req protocol.Create, quick bool) (protocol.Credentials, error) {
	name, err := nickname(req.Nickname)
	if err != nil || !req.Mode.Valid() || req.Capacity < 2 || req.Capacity > 5 {
		return protocol.Credentials{}, errInput
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.rooms) >= d.cfg.MaxRooms || d.ctx.Err() != nil {
		return protocol.Credentials{}, errBusy
	}
	var id string
	for i := 0; i < 16; i++ {
		n, err := card.SecureIndex(1000000)
		if err != nil {
			return protocol.Credentials{}, errBusy
		}
		id = fmt.Sprintf("%06d", n)
		if d.rooms[id] == nil {
			break
		}
		id = ""
	}
	if id == "" {
		return protocol.Credentials{}, errBusy
	}
	ctx, cancel := context.WithCancel(d.ctx)
	r := &Room{id: id, mode: req.Mode, capacity: req.Capacity, players: make([]*member, req.Capacity), dir: d, queue: make(chan func(), 64), ctx: ctx, cancel: cancel, offlineSince: time.Now(), quick: quick}
	cred, err := r.addHuman(name)
	if err != nil {
		cancel()
		return protocol.Credentials{}, err
	}
	if quick {
		for i := 1; i < req.Capacity; i++ {
			if err := r.addBot(); err != nil {
				cancel()
				return protocol.Credentials{}, err
			}
		}
	}
	d.rooms[id] = r
	d.wg.Add(1)
	go func() { defer d.wg.Done(); r.run() }()
	return cred, nil
}

func (d *Directory) Join(ctx context.Context, req protocol.Join) (protocol.Credentials, error) {
	name, err := nickname(req.Nickname)
	if err != nil || !roomID.MatchString(req.RoomID) {
		return protocol.Credentials{}, errInput
	}
	r := d.Find(req.RoomID)
	if r == nil {
		return protocol.Credentials{}, errMissing
	}
	type result struct {
		credentials protocol.Credentials
		err         error
	}
	reply := make(chan result, 1)
	if !r.try(func() {
		if ctx.Err() != nil {
			reply <- result{err: errBusy}
			return
		}
		if r.game != nil {
			reply <- result{err: errPhase}
			return
		}
		c, err := r.addHuman(name)
		if err == nil {
			r.broadcast(nil)
		}
		reply <- result{c, err}
	}) {
		return protocol.Credentials{}, errBusy
	}
	select {
	case response := <-reply:
		return response.credentials, response.err
	case <-ctx.Done():
		return protocol.Credentials{}, errBusy
	case <-r.ctx.Done():
		return protocol.Credentials{}, errMissing
	}
}
