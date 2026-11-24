package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/duanap/gandengyan/server/internal/game"
	"github.com/duanap/gandengyan/server/internal/simulation"
)

func main() {
	games := flag.Int("per-group", 1250, "games in each selected group")
	first := flag.Int64("seed", 13001, "first reproducible seed in each group")
	players := flag.Int("players", 0, "player count, 0 for all")
	mode := flag.String("mode", "", "classic or speed, empty for both")
	flag.Parse()
	if *games < 1 || *players != 0 && (*players < 2 || *players > 5) || *mode != "" && !game.Mode(*mode).Valid() {
		log.Fatal("invalid arguments")
	}
	out := bufio.NewWriter(os.Stdout)
	encoder := json.NewEncoder(out)
	for n := 2; n <= 5; n++ {
		for _, m := range []game.Mode{game.Classic, game.Speed} {
			if *players != 0 && n != *players || *mode != "" && game.Mode(*mode) != m {
				continue
			}
			sum := simulation.Record{}
			depleted := 0
			for i := 0; i < *games; i++ {
				record, err := simulation.Run(*first+int64(i), n, m)
				if writeErr := encoder.Encode(record); writeErr != nil {
					log.Fatal(writeErr)
				}
				if err != nil {
					_ = out.Flush()
					log.Fatalf("seed=%d players=%d mode=%s action=%d: %v", record.Seed, n, m, record.Actions, err)
				}
				sum.Actions += record.Actions
				sum.Bombs += record.Bombs
				sum.Passes += record.Passes
				sum.Wildcards += record.Wildcards
				if record.Depleted {
					depleted++
				}
			}
			fmt.Fprintf(os.Stderr, "%d %s: games=%d actions=%d passes=%d bombs=%d wildcards=%d depleted=%d\n", n, m, *games, sum.Actions, sum.Passes, sum.Bombs, sum.Wildcards, depleted)
		}
	}
	if err := out.Flush(); err != nil {
		log.Fatal(err)
	}
}
