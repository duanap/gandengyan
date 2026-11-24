package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"runtime/metrics"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/protocol"
	"github.com/duanap/gandengyan/server/internal/room"
	"github.com/duanap/gandengyan/server/internal/rules"
	"github.com/gorilla/websocket"
)

func TestLoopbackLoad(t *testing.T) {
	if os.Getenv("GDY_LOAD") == "" {
		t.Skip("set GDY_LOAD=1 to run the sustained socket load test")
	}
	const rooms = 48
	dir := room.NewDirectory(t.Context(), room.Config{MaxRooms: rooms + 1, OfflineTTL: 1400 * time.Millisecond, SessionTTL: time.Hour, AIWorkers: 2})
	defer dir.Close()
	server := httptest.NewServer(New(dir, Config{MaxConnections: 2 * rooms, MaxPerIP: 4, TrustLoopbackProxy: true}, nil))
	defer server.Close()
	results := make([][]time.Duration, rooms)
	start := time.Now()
	samples := []metrics.Sample{{Name: "/cpu/classes/user:cpu-seconds"}}
	metrics.Read(samples)
	startCPU := samples[0].Value.Float64()
	t.Run("rooms", func(parent *testing.T) {
		for i := 0; i < rooms; i++ {
			parent.Run(fmt.Sprint(i+1), func(t *testing.T) {
				t.Parallel()
				results[i] = loadRoom(t, server.URL, i)
			})
		}
	})
	total := time.Since(start)
	metrics.Read(samples)
	userCPU := samples[0].Value.Float64() - startCPU
	var latency []time.Duration
	for _, l := range results {
		latency = append(latency, l...)
	}
	if len(latency) < 1000 {
		t.Fatalf("insufficient successful actions: %d", len(latency))
	}
	slices.Sort(latency)
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	t.Logf("rooms=%d clients=%d duration=%s actions=%d rate=%.0f/s p50=%s p95=%s p99=%s max=%s Go_user_CPU_s=%.2f alloc_MiB=%.2f sys_MiB=%.2f",
		rooms, rooms*2, total, len(latency), float64(len(latency))/total.Seconds(), latency[len(latency)/2], latency[len(latency)*95/100],
		latency[len(latency)*99/100], latency[len(latency)-1], userCPU, float64(mem.Alloc)/(1<<20), float64(mem.Sys)/(1<<20))
	limit := time.Now().Add(5 * time.Second)
	for dir.Count() > 0 && time.Now().Before(limit) {
		time.Sleep(50 * time.Millisecond)
	}
	if dir.Count() != 0 {
		t.Fatal("rooms were not reclaimed", dir.Count())
	}
	t.Log("all 48 rooms reclaimed after disconnect")
}

func loadCred(t *testing.T, base, path, ip string, body any) protocol.Credentials {
	t.Helper()
	encoded, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", ip)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 201 {
		t.Fatalf("HTTP %d", res.StatusCode)
	}
	var c protocol.Credentials
	if err := json.NewDecoder(res.Body).Decode(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func loadRoom(t *testing.T, base string, index int) []time.Duration {
	t.Helper()
	ip := fmt.Sprintf("192.0.2.%d", index+1)
	mode := game.Classic
	if index%2 == 1 {
		mode = game.Speed
	}
	ca := loadCred(t, base, "/api/rooms", ip, protocol.Create{Nickname: "player a", Capacity: 2, Mode: mode})
	cb := loadCred(t, base, "/api/rooms/join", ip, protocol.Join{RoomID: ca.RoomID, Nickname: "player b"})
	credentials := []protocol.Credentials{ca, cb}
	clients := make([]*websocket.Conn, 2)
	for i, cred := range credentials {
		c, res, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(base, "http")+"/ws", http.Header{"Origin": {base}, "X-Real-IP": {ip}})
		if err != nil {
			if res != nil {
				res.Body.Close()
			}
			t.Fatal(err)
		}
		defer c.Close()
		clients[i] = c
		send(t, c, command(packet{}, "auth", "auth", protocol.Auth{RoomID: cred.RoomID, SessionToken: cred.SessionToken}))
	}
	ready := readUntil(t, clients[0], func(p packet) bool {
		return p.Type == "room_state" && allOnline(decode[protocol.RoomState](t, p.Payload))
	})
	send(t, clients[0], command(ready, "start_game", "start", struct{}{}))
	ack(t, clients[0], "start", "")
	states := []packet{receive(t, clients[0], "game_state"), receive(t, clients[1], "game_state")}
	var latencies []time.Duration
	for i := 0; i < 60; i++ {
		state := decode[protocol.GameState](t, states[0].Payload)
		seat := 0
		kind := "restart_game"
		var payload any = struct{}{}
		if state.Phase != game.Finished {
			if state.CurrentID == cb.PlayerID {
				seat = 1
			}
			kind = "pass"
			send(t, clients[seat], command(states[seat], "hint", fmt.Sprint("hint", i), struct{}{}))
			hint := decode[*rules.Pattern](t, receive(t, clients[seat], "play_hint").Payload)
			if hint != nil {
				kind = "play_cards"
				payload = rules.Request{CardIDs: hint.CardIDs, Pattern: &rules.Declaration{Shape: hint.Shape}}
			}
		}
		id := fmt.Sprint("action", i)
		now := time.Now()
		send(t, clients[seat], command(states[seat], kind, id, payload))
		ack(t, clients[seat], id, "")
		for i, c := range clients {
			states[i] = receive(t, c, "game_state")
		}
		latencies = append(latencies, time.Since(now))
		for i, p := range states {
			view := decode[protocol.GameState](t, p.Payload)
			if view.RoomID != ca.RoomID || view.SelfID != credentials[i].PlayerID || p.Revision != states[0].Revision {
				t.Fatal("cross-room or revision mismatch")
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return latencies
}
