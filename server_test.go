package main

import (
	"os"
	"strconv"
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
	s.Init()

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

	n := "test"

	def := NewDef()
	def.Name = n
	def.Type = "seq"
	def.Offset = 100000

	if err := s.CreateDef(def); err != nil {
		t.Fatal(err)
	}

	idents := []*IdentAlias{
		{Ident: "a"},
		{Ident: "b"},
		{Ident: "c"},
		{Ident: "d"},
		{Ident: "e"},
		{Ident: "f"},
	}

	idents, err := s.Gen(def, idents)
	if err != nil {
		t.Fatal(err)
	}

	for _, ia := range idents {
		if ia.Alias == "" || ia.Status != StatusCreated {
			t.Errorf("%s alias failed", ia.Ident)
		}
	}

	idents, err = s.Get(def, idents)
	if err != nil {
		t.Fatal(err)
	}

	for _, ia := range idents {
		if ia.Alias == "" || ia.Status != StatusExists {
			t.Errorf("%s alias missing", ia.Ident)
		}
	}
}
