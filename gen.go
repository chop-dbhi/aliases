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
	// RandMinlen is the default alias length for random alias generators.
	RandMinlen = 8
	// RandChars is the default character set for random alias generators.
	RandChars = "abcdefghijklmnopqrstuvwzyz0123456789"

	// MinRandMinlen is the minimum alias length allowed for random alias generators.
	MinRandMinlen = 4
	// MinRandChars is the minimum number of characters allowed in a random alias
	// generator character set.
	MinRandChars = 8
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Def is an alias generator definition.
type Def struct {
	// Internal ID of the definition.
	ID int `json:"id"`

	// Name of the generator.
	Name string `json:"name"`

	// Type of generator.
	Type string `json:"type"`

	// Offset for seq generator.
	// **NOT IMPLEMENTED**
	Offset int64 `json:"offset"`

	// Apply to rand generator.
	Chars  string `json:"chars"`
	Minlen int    `json:"minlen"`
	Prefix string `json:"prefix"`

	// Whether the definition is archived or not.
	Deleted bool `json:"archived"`
}

// NewDef returns a new alias generator definition with the default settings.
func NewDef() *Def {
	return &Def{
		Chars:  RandChars,
		Minlen: RandMinlen,
	}
}

// MakeGen makes an alias generator from the given definition given a redis connection.
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

// Gen is an alias generator interface.
type Gen interface {
	New() (string, error)
}

// UUIDGen generates random UUIDs.
type UUIDGen struct{}

// New generates a new random UUID.
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

// New generates a new random alias.
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

// SeqGen is a sequential alias generator.
type SeqGen struct {
	Name   string
	Offset int64

	conn redis.Conn
}

// New generates a new sequential alias.
func (g *SeqGen) New() (string, error) {
	id, err := redis.Int64(g.conn.Do("INCR", seqPrefix+g.Name))
	if err != nil && err != redis.ErrNil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}
