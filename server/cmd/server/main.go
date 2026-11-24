package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duanap/gandengyan/server/internal/gateway"
	"github.com/duanap/gandengyan/server/internal/protocol"
	"github.com/duanap/gandengyan/server/internal/room"
	"github.com/duanap/gandengyan/server/internal/webassets"
)

var build = "dev"

type config struct {
	Listen         string   `json:"listen"`
	Origins        []string `json:"allowedOrigins"`
	TrustProxy     bool     `json:"trustLoopbackProxy"`
	MaxRooms       int      `json:"maxRooms"`
	MaxConnections int      `json:"maxConnections"`
	MaxPerIP       int      `json:"maxConnectionsPerIP"`
	IdleMinutes    int      `json:"idleOfflineRoomMinutes"`
	SessionHours   int      `json:"sessionHours"`
	AIWorkers      int      `json:"aiWorkers"`
}

func load(path string) (config, error) {
	c := config{Listen: "127.0.0.1:8080", MaxRooms: 512, MaxConnections: 1024, MaxPerIP: 20, IdleMinutes: 30, SessionHours: 24, AIWorkers: 2}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return c, err
		}
		defer f.Close()
		if err := protocol.Read(f, &c); err != nil {
			return c, err
		}
	}
	if c.Listen == "" || c.MaxRooms < 1 || c.MaxConnections < 1 || c.MaxPerIP < 1 || c.IdleMinutes < 1 || c.IdleMinutes > 1440 || c.SessionHours < 1 || c.SessionHours > 168 || c.AIWorkers < 1 || c.AIWorkers > 64 {
		return c, fmt.Errorf("invalid server configuration")
	}
	for _, origin := range c.Origins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || origin != u.Scheme+"://"+u.Host {
			return c, fmt.Errorf("allowedOrigins must contain exact HTTP(S) origins")
		}
	}
	return c, nil
}

func main() {
	path := flag.String("config", "", "path to JSON server config")
	listen := flag.String("listen", "", "override listen address")
	version := flag.Bool("version", false, "show build version")
	flag.Parse()
	if *version {
		fmt.Printf("gandengyan %s rules=1.3 protocol=%d\n", build, protocol.Version)
		return
	}
	c, err := load(*path)
	if err != nil {
		log.Fatal(err)
	}
	if *listen != "" {
		c.Listen = *listen
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rooms := room.NewDirectory(ctx, room.Config{MaxRooms: c.MaxRooms, OfflineTTL: time.Duration(c.IdleMinutes) * time.Minute, SessionTTL: time.Duration(c.SessionHours) * time.Hour, AIWorkers: c.AIWorkers})
	defer rooms.Close()
	assets, embedded := webassets.Files()
	handler := gateway.New(rooms, gateway.Config{Origins: c.Origins, TrustLoopbackProxy: c.TrustProxy, MaxConnections: c.MaxConnections, MaxPerIP: c.MaxPerIP, Build: build}, assets)
	server := &http.Server{Addr: c.Listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("gandengyan build=%s listen=%s embedded=%t", build, c.Listen, embedded)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("listen failed: %v", err)
		os.Exit(1)
	}
}
