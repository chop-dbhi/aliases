package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"
)

func initServer(t testing.TB) *Server {
	var (
		db  int
		err error
	)

	if os.Getenv("REDIS_DB") != "" {
		db, err = strconv.Atoi(os.Getenv("REDIS_DB"))
		if err != nil {
			t.Fatal(err)
		}
	}

	s := &Server{
		RedisAddr: os.Getenv("REDIS_ADDR"),
		RedisDB:   int(db),
	}

	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	// Flush the DB.
	c := s.Pool.Get()
	defer c.Close()
	if _, err = c.Do("FLUSHDB"); err != nil {
		t.Fatal(err)
	}

	return s
}

func TestServer(t *testing.T) {
	s := initServer(t)

	var (
		n = "test"
		r bytes.Buffer
		w bytes.Buffer
	)

	def := NewDef()
	def.Name = n
	def.Type = "seq"
	def.Offset = 100000

	if err := s.CreateDef(def); err != nil {
		t.Fatal(err)
	}

	r.Reset()
	r.WriteString("a\nb\nc\nd\ne\nf")

	if err := s.Gen(def, &r, &w); err != nil {
		t.Fatal(err)
	}

	b, err := ioutil.ReadAll(&w)
	if err != nil {
		t.Fatal(err)
	}

	as := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(as) != 6 {
		t.Errorf("expected 6 aliases, got %d", len(as))
	}
}
