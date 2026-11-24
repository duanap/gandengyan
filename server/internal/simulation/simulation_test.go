package simulation

import (
	"github.com/duanap/gandengyan/server/internal/game"
	"testing"
)

func TestMatrix(t *testing.T) {
	for n := 2; n <= 5; n++ {
		for _, mode := range []game.Mode{game.Classic, game.Speed} {
			for seed := int64(1); seed <= 32; seed++ {
				if result, err := Run(seed, n, mode); err != nil {
					t.Fatalf("%+v: %v", result, err)
				}
			}
		}
	}
}
