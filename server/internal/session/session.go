package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	"github.com/duanap/gandengyan/server/internal/protocol"
)

func Token(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func Digest(token string) [32]byte { return sha256.Sum256([]byte(token)) }

// Peer is a bounded mailbox. The room is its sole writer and owns seat
// binding. The transport can read it without acquiring room state.
type Peer struct {
	Out  chan protocol.Message
	Stop chan int
}

func NewPeer() *Peer {
	return &Peer{Out: make(chan protocol.Message, 16), Stop: make(chan int, 1)}
}
func (p *Peer) Close(code int) {
	select {
	case p.Stop <- code:
	default:
	}
}

const (
	Replaced       = 4001
	InvalidSession = 4003
	Left           = 4004
	Retry          = 1013
)
