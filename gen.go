package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/garyburd/redigo/redis"
	uuid "github.com/satori/go.uuid"
)

var (
	RandMinlen = 8
	RandChars  = "abcdefghijklmnopqrstuvwzyz0123456789"

	MinRandMinlen = 4
	MinRandChars  = 8
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Def struct {
	// Internal ID of the definition.
	ID int `json:"id"`

	// Name of the generator.
	Name string `json:"name"`

	// Type of generator.
	Type string `json:"type"`

	// Offset for seq generator.
	Offset int64 `json:"offset"`

	// Apply to rand generator.
	Chars  string `json:"chars"`
	Minlen int    `json:"minlen"`
	Prefix string `json:"prefix"`

	Deleted bool `json:"archived"`
}

func NewDef() *Def {
	return &Def{
		Chars:  RandChars,
		Minlen: RandMinlen,
	}
}

func MakeGen(c redis.Conn, d *Def) Gen {
	switch d.Type {
	case "uuid":
		return &UUIDGen{}

	case "rand":
		return &RandGen{
			Prefix:  d.Prefix,
			Minlen:  d.Minlen,
			Chars:   d.Chars,
			charlen: len(d.Chars),
		}

	case "seq":
		return &SeqGen{
			Name:   d.Name,
			Offset: d.Offset,
			conn:   c,
		}
	}

	return nil
}

type Gen interface {
	New() (string, error)
}

// UUIDGen generates random UUIDs.
type UUIDGen struct{}

func (g *UUIDGen) New() (string, error) {
	return uuid.NewV4().String(), nil
}

// RandGen is a random alias generator.
type RandGen struct {
	Prefix string
	Minlen int
	Chars  string

	charlen int
}

func (g *RandGen) New() (string, error) {
	key := make([]byte, g.Minlen)

	for i := range key {
		key[i] = g.Chars[rand.Intn(g.charlen)]
	}

	alias := string(key)

	if g.Prefix == "" {
		return alias, nil
	}

	return fmt.Sprintf("%s%s", g.Prefix, alias), nil
}

type SeqGen struct {
	Name   string
	Offset int64

	conn redis.Conn
}

func (g *SeqGen) New() (string, error) {
	id, err := redis.Int64(g.conn.Do("INCR", seqPrefix+g.Name))
	if err != nil && err != redis.ErrNil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}
