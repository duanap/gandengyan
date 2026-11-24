package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/duanap/gandengyan/server/internal/protocol"
	"github.com/duanap/gandengyan/server/internal/room"
	"github.com/duanap/gandengyan/server/internal/session"
)

type Config struct {
	Origins            []string
	TrustLoopbackProxy bool
	MaxConnections     int
	MaxPerIP           int
	Build              string
}

type Gateway struct {
	rooms       *room.Directory
	cfg         Config
	mux         *http.ServeMux
	upgrader    websocket.Upgrader
	mu          sync.Mutex
	connections int
	perIP       map[string]int
	limits      map[string]bucket
	lastCleanup time.Time
}
type bucket struct {
	tokens  float64
	updated time.Time
}

func (b *bucket) take(now time.Time, burst, perSecond float64) bool {
	if b.updated.IsZero() {
		b.tokens = burst
	} else {
		b.tokens = min(burst, b.tokens+now.Sub(b.updated).Seconds()*perSecond)
	}
	b.updated = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func New(rooms *room.Directory, cfg Config, assets fs.FS) *Gateway {
	g := &Gateway{rooms: rooms, cfg: cfg, mux: http.NewServeMux(), perIP: make(map[string]int), limits: make(map[string]bucket)}
	g.upgrader = websocket.Upgrader{CheckOrigin: g.originOK, HandshakeTimeout: 5 * time.Second, ReadBufferSize: 1024, WriteBufferSize: 4096, WriteBufferPool: new(sync.Pool)}
	g.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { sendJSON(w, 200, map[string]string{"status": "ok"}) })
	g.mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		sendJSON(w, 200, map[string]any{"build": cfg.Build, "protocolVersion": protocol.Version, "rulesVersion": "1.3"})
	})
	g.mux.HandleFunc("POST /api/rooms", g.create(false))
	g.mux.HandleFunc("POST /api/rooms/quick", g.create(true))
	g.mux.HandleFunc("POST /api/rooms/join", g.join)
	g.mux.HandleFunc("GET /ws", g.websocket)
	if assets != nil {
		files := http.FileServerFS(assets)
		g.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/":
				w.Header().Set("Cache-Control", "no-cache")
			case strings.HasPrefix(r.URL.Path, "/assets/"):
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			default:
				w.Header().Set("Cache-Control", "no-cache")
			}
			files.ServeHTTP(w, r)
		})
	}
	return g
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	g.mux.ServeHTTP(w, r)
}

func (g *Gateway) originOK(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if len(g.cfg.Origins) > 0 {
		for _, allowed := range g.cfg.Origins {
			if origin == allowed {
				return true
			}
		}
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+r.Host
}
func (g *Gateway) ip(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	addr, _ := netip.ParseAddr(host)
	if g.cfg.TrustLoopbackProxy && addr.IsLoopback() {
		if forwarded, err := netip.ParseAddr(r.Header.Get("X-Real-IP")); err == nil {
			return forwarded.Unmap().String()
		}
	}
	return addr.Unmap().String()
}
func (g *Gateway) allowIP(key string, burst, rate float64) bool {
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	if now.Sub(g.lastCleanup) > time.Minute {
		for ip, b := range g.limits {
			if now.Sub(b.updated) > 2*time.Minute {
				delete(g.limits, ip)
			}
		}
		g.lastCleanup = now
	}
	b, known := g.limits[key]
	if !known && len(g.limits) >= 16384 {
		return false
	}
	ok := b.take(now, burst, rate)
	g.limits[key] = b
	return ok
}
func (g *Gateway) reserve(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.connections >= g.cfg.MaxConnections || g.perIP[ip] >= g.cfg.MaxPerIP {
		return false
	}
	g.connections++
	g.perIP[ip]++
	return true
}
func (g *Gateway) release(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.connections--
	g.perIP[ip]--
	if g.perIP[ip] == 0 {
		delete(g.perIP, ip)
	}
}

func sendJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func reject(w http.ResponseWriter, status int, code string) {
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "2")
	}
	sendJSON(w, status, protocol.Issue(code))
}
func requestError(w http.ResponseWriter, err error) {
	status := http.StatusConflict
	switch err.Error() {
	case "INVALID_INPUT":
		status = 400
	case "ROOM_NOT_FOUND":
		status = 404
	case "SERVER_BUSY":
		status = 503
	}
	reject(w, status, err.Error())
}

func (g *Gateway) readRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.Header.Get("Origin") != "" && !g.originOK(r) {
		reject(w, 403, "UNAUTHORIZED")
		return false
	}
	if !g.allowIP("rooms:"+g.ip(r), 20, 0.5) {
		reject(w, 429, "RATE_LIMITED")
		return false
	}
	m, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if m != "application/json" {
		reject(w, 415, "INVALID_INPUT")
		return false
	}
	defer r.Body.Close()
	if err := protocol.Read(http.MaxBytesReader(w, r.Body, 2048), target); err != nil {
		status := 400
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			status = 413
		}
		reject(w, status, "INVALID_INPUT")
		return false
	}
	return true
}

func (g *Gateway) create(quick bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req protocol.Create
		if !g.readRequest(w, r, &req) {
			return
		}
		c, err := g.rooms.Create(req, quick)
		if err != nil {
			requestError(w, err)
			return
		}
		sendJSON(w, 201, c)
	}
}
func (g *Gateway) join(w http.ResponseWriter, r *http.Request) {
	var req protocol.Join
	if !g.readRequest(w, r, &req) {
		return
	}
	c, err := g.rooms.Join(r.Context(), req)
	if err != nil {
		requestError(w, err)
		return
	}
	sendJSON(w, 200, c)
}

func validCommand(command protocol.Command) bool {
	return len(command.Type) > 0 && len(command.Type) <= 32 && len(command.RequestID) > 0 && len(command.RequestID) <= 80 && len(command.GameID) <= 64
}

func read(c *websocket.Conn) (protocol.Command, error) {
	var command protocol.Command
	kind, data, err := c.ReadMessage()
	if err != nil {
		return command, err
	}
	if kind != websocket.TextMessage || protocol.Decode(data, &command) != nil || !validCommand(command) {
		return command, errors.New("invalid command")
	}
	return command, nil
}

const (
	maxFrame    = 32 * 1024
	readWindow  = 65 * time.Second
	writeWindow = 5 * time.Second
)

func closeFrame(c *websocket.Conn, code int) {
	_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, ""), time.Now().Add(writeWindow))
}

func (g *Gateway) websocket(w http.ResponseWriter, r *http.Request) {
	if !g.originOK(r) {
		reject(w, 403, "UNAUTHORIZED")
		return
	}
	ip := g.ip(r)
	if !g.allowIP("ws:"+ip, 40, 2) || !g.reserve(ip) {
		reject(w, 429, "RATE_LIMITED")
		return
	}
	defer g.release(ip)
	c, err := g.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	c.SetReadLimit(maxFrame)
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	command, err := read(c)
	if err != nil || command.Type != "auth" {
		closeFrame(c, session.InvalidSession)
		return
	}
	if command.V != protocol.Version {
		closeFrame(c, 4006)
		return
	}
	var auth protocol.Auth
	if command.Parse(&auth) != nil || len(auth.RoomID) != 6 || len(auth.SessionToken) != 43 {
		closeFrame(c, session.InvalidSession)
		return
	}
	room := g.rooms.Find(auth.RoomID)
	if room == nil {
		closeFrame(c, session.InvalidSession)
		return
	}
	peer := session.NewPeer()
	if !room.Attach(peer, auth.SessionToken, command.RequestID) {
		closeFrame(c, session.Retry)
		return
	}
	defer room.Detach(peer)
	_ = c.SetReadDeadline(time.Now().Add(readWindow))
	c.SetPongHandler(func(string) error { return c.SetReadDeadline(time.Now().Add(readWindow)) })
	ctx, cancel := context.WithCancel(r.Context())
	finished := make(chan struct{})
	go func() { defer close(finished); defer c.Close(); write(ctx, c, peer) }()
	defer func() { cancel(); <-finished }()
	var commands bucket
	for {
		command, err := read(c)
		if err != nil {
			return
		}
		if command.V != protocol.Version {
			closeFrame(c, 4006)
			return
		}
		if !commands.take(time.Now(), 24, 12) || !room.Submit(peer, command) {
			closeFrame(c, session.Retry)
			return
		}
	}
}
func write(ctx context.Context, c *websocket.Conn, p *session.Peer) {
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		// Let a replacement or a backlogged client close before queued snapshots.
		select {
		case code := <-p.Stop:
			closeFrame(c, code)
			return
		default:
		}
		select {
		case <-ctx.Done():
			return
		case code := <-p.Stop:
			closeFrame(c, code)
			return
		case msg := <-p.Out:
			_ = c.SetWriteDeadline(time.Now().Add(writeWindow))
			if c.WriteJSON(msg) != nil {
				return
			}
			if msg.CloseAfter != 0 {
				closeFrame(c, msg.CloseAfter)
				return
			}
		case <-heartbeat.C:
			if c.WriteControl(websocket.PingMessage, []byte(strconv.FormatInt(time.Now().Unix(), 10)), time.Now().Add(writeWindow)) != nil {
				return
			}
		}
	}
}
