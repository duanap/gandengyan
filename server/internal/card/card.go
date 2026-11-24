package card

import (
	cryptorand "crypto/rand"
	"errors"
	"math/big"
	"slices"
	"strconv"
)

type ID int

func (id *ID) UnmarshalJSON(data []byte) error {
	n, err := strconv.Atoi(string(data))
	if err != nil {
		return errors.New("card ID must be an integer")
	}
	*id = ID(n)
	return nil
}

const (
	Three         = 3
	Ace           = 14
	Two           = 15
	SmallJoker ID = 52
	BigJoker   ID = 53
	Count         = 54
)

func (id ID) Valid() bool { return id >= 0 && id < Count }
func (id ID) Rank() int {
	if !id.Valid() || id >= SmallJoker {
		return 0
	}
	return int(id)/4 + Three
}

func Deck() []ID {
	deck := make([]ID, Count)
	for i := range deck {
		deck[i] = ID(i)
	}
	return deck
}

func SecureIndex(n int) (int, error) {
	index, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(index.Int64()), nil
}

func Shuffle(index func(int) (int, error)) ([]ID, error) {
	deck := Deck()
	for i := len(deck) - 1; i > 0; i-- {
		j, err := index(i + 1)
		if err != nil {
			return nil, err
		}
		deck[i], deck[j] = deck[j], deck[i]
	}
	return deck, nil
}

func Sorted(ids []ID) []ID {
	out := append([]ID{}, ids...)
	slices.Sort(out)
	return out
}
