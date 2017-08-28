package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/garyburd/redigo/redis"
)

var (
	DefaultIdleTimeout = 5 * time.Minute
	DefaultMaxIdle     = 3

	ErrNoDef      = errors.New("no def")
	ErrDefExists  = errors.New("def exists")
	ErrBadDefName = errors.New("name may only contain [A-Za-z0-9-_.] chars")

	MaxAttempts           = 100
	ErrMaxAttemptsReached = errors.New("max attempts reached")

	nameRegex  *regexp.Regexp
	splitRegex *regexp.Regexp

	// Prefix for internal use.
	internalPrefix = "_:%s"

	// Prefix for index definitions.
	defPrefix   = "d:%s"
	valuePrefix = "v:%d"
	seqPrefix   = "s:%d"

	// Prefix for keys, aliases, and sequences.
	// These are scoped by the definition id.
	keyPrefix   = "k:%d:%s"
	aliasPrefix = "a:%d:%s"
)

func mk(f string, v ...interface{}) string {
	return fmt.Sprintf(f, v...)
}

func init() {
	nameRegex = regexp.MustCompile(`^[A-Za-z0-9-_\.]+$`)
	splitRegex = regexp.MustCompile(`[\s,\t]+`)
}

const (
	StatusExists = Status(iota + 1)
	StatusCreated
	StatusMissing
)

type Status int

func (s Status) String() string {
	switch s {
	case StatusExists:
		return "exists"
	case StatusCreated:
		return "created"
	case StatusMissing:
		return "missing"
	}
	return ""
}

func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

type IdentAlias struct {
	Ident  string `json:"ident"`
	Alias  string `json:"alias,omitempty"`
	Status Status `json:"status,omitempty"`
}

type Server struct {
	RedisAddr string
	RedisDB   int
	RedisPass string

	Log  *log.Logger
	Pool *redis.Pool
}

func (s *Server) Close() {
	if s.Pool != nil {
		s.Pool.Close()
	}
}

func (s *Server) Init() error {
	s.Log = log.New(os.Stderr, "aliases: ", 0)

	// Create a pool of Redis connections.
	s.Pool = &redis.Pool{
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", s.RedisAddr)
			if err != nil {
				return nil, err
			}

			// Password to authenticate.
			if s.RedisPass != "" {
				if _, err := conn.Do("AUTH", s.RedisPass); err != nil {
					return nil, err
				}
			}

			if _, err := conn.Do("SELECT", s.RedisDB); err != nil {
				return nil, err
			}

			return conn, nil
		},
		IdleTimeout: DefaultIdleTimeout,
		MaxIdle:     DefaultMaxIdle,
	}

	return nil
}

func (s *Server) GetDefs() ([]json.RawMessage, error) {
	conn := s.Pool.Get()
	defer conn.Close()

	// Keys of the definitions.
	keys, err := redis.Strings(conn.Do("KEYS", "v:*"))
	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return []json.RawMessage{}, nil
	}

	args := make([]interface{}, len(keys))
	for i, k := range keys {
		args[i] = k
	}

	vals, err := redis.Strings(conn.Do("MGET", args...))
	if err != nil {
		return nil, err
	}

	defs := make([]json.RawMessage, len(vals))

	for i, val := range vals {
		defs[i] = json.RawMessage(val)
	}

	return defs, nil
}

// DelDef marks a index for deletion.
func (s *Server) DelDef(name string) error {
	def, err := s.GetDef(name)
	if err != nil {
		return err
	}

	// Internally mark as deleted to be cleaned up.
	def.Deleted = true
	b, err := json.Marshal(def)
	if err != nil {
		return err
	}

	conn := s.Pool.Get()
	defer conn.Close()

	// Delete name entry to make inaccessable and update definition.
	conn.Send("MULTI")
	conn.Send("DEL", mk(defPrefix, name))
	conn.Send("SET", mk(valuePrefix, def.ID), string(b))
	if _, err := conn.Do("EXEC"); err != nil {
		return err
	}

	s.Log.Printf("deleted '%s'", def.Name)

	return nil
}

func (s *Server) GetDef(name string) (*Def, error) {
	conn := s.Pool.Get()
	defer conn.Close()

	id, err := redis.Int64(conn.Do("GET", mk(defPrefix, name)))
	if err == redis.ErrNil {
		return nil, ErrNoDef
	} else if err != nil {
		return nil, err
	}

	blob, err := redis.Bytes(conn.Do("GET", mk(valuePrefix, id)))
	if err != nil {
		return nil, err
	}

	if blob == nil {
		panic(fmt.Sprintf("missing def value for %s", name))
	}

	var g Def
	if err := json.Unmarshal(blob, &g); err != nil {
		return nil, err
	}

	return &g, nil
}

func (s *Server) validateDef(def *Def) error {
	if def.Name == "" {
		return errors.New("name required")
	}

	if !nameRegex.MatchString(def.Name) {
		return ErrBadDefName
	}

	if def.Type == "" {
		return errors.New("type required")
	}

	switch def.Type {
	case "seq":
	case "rand":
		if def.Minlen < MinRandMinlen {
			return errors.New("rand min length too small")
		}

		if len(def.Chars) < MinRandChars {
			return errors.New("too few chars for rand")
		}
	case "uuid":
	default:
		return errors.New("unknown type")
	}

	return nil
}

// CreateDef creates a new index for generating aliases.
// d:foo -> 0
// v:0 -> { ... }
func (s *Server) CreateDef(def *Def) error {
	if err := s.validateDef(def); err != nil {
		return err
	}

	// Check if there is an existing definition.
	conn := s.Pool.Get()
	defer conn.Close()

	// Lookup up def by name.
	defKey := mk(defPrefix, def.Name)

	exists, err := redis.Bool(conn.Do("EXISTS", defKey))
	if err != nil {
		return err
	}

	// Cannot create a def by the same name.
	if exists {
		return ErrDefExists
	}

	// Get a new key.
	defIdKey := mk(internalPrefix, "def:id")
	id, err := redis.Int64(conn.Do("INCR", defIdKey))
	if err != nil {
		return err
	}

	def.ID = int(id)

	b, err := json.Marshal(def)
	if err != nil {
		return err
	}

	valueKey := mk(valuePrefix, def.ID)

	args := []interface{}{
		defKey, def.ID,
		valueKey, string(b),
	}

	// Initialize the sequence.
	if def.Type == "seq" {
		seqKey := mk(seqPrefix, def.ID)
		args = append(args, seqKey, def.Offset)
	}

	_, err = conn.Do("MSET", args...)
	if err != nil {
		return err
	}

	s.Log.Printf("created def '%s' (id=%d)", def.Name, def.ID)

	return nil
}

func (s *Server) UpdateDef(name string, def *Def) error {
	if err := s.validateDef(def); err != nil {
		return err
	}

	// Check if there is an existing definition.
	conn := s.Pool.Get()
	defer conn.Close()

	b, err := json.Marshal(def)
	if err != nil {
		return err
	}

	// Delete previous definition.
	if name != def.Name {
		_, err = conn.Do("DEL", mk(defPrefix, name))
		if err != nil {
			return err
		}
	}

	// Set name and value key.
	defKey := mk(defPrefix, def.Name)
	valueKey := mk(valuePrefix, def.ID)
	_, err = conn.Do("MSET", defKey, def.ID, valueKey, string(b))
	if err != nil {
		return err
	}

	s.Log.Printf("updated def '%s'", def.Name)

	return nil
}

func (s *Server) Gen(def *Def, idents []*IdentAlias) ([]*IdentAlias, error) {
	conn := s.Pool.Get()
	defer conn.Close()

	// Generator for this line.
	gen := MakeGen(conn, def)

	for _, ia := range idents {
		if ia.Ident == "" {
			continue
		}

		lookupKey := mk(keyPrefix, def.ID, ia.Ident)

		// Check if the key already exists. If so, just return it.
		alias, err := redis.String(conn.Do("GET", lookupKey))

		// Exists.
		if err == nil {
			ia.Alias = alias
			ia.Status = StatusExists
			continue
		}

		if err != nil && err != redis.ErrNil {
			return nil, err
		}

		var attempt int

		for {
			if attempt == MaxAttempts {
				s.Log.Printf("max attempts reached for '%s' in '%s'", lookupKey, def.Name)
				// TODO: auto-increase minlenth if this occurs.
				return nil, ErrMaxAttemptsReached
			}

			attempt++

			// Generate new key.
			alias, err = gen.New()
			if err != nil {
				return nil, err
			}

			// Check if it exists, otherwise set it.
			checkKey := mk(aliasPrefix, def.ID, alias)

			ok, err := redis.Bool(conn.Do("EXISTS", checkKey))
			if err != nil {
				return nil, err
			}

			// Does not exist, set it.
			if !ok {
				_, err := conn.Do("MSET", lookupKey, alias, checkKey, true)
				if err != nil {
					return nil, err
				}

				ia.Alias = alias
				ia.Status = StatusCreated

				// TODO: add metric for number of attempts. this is an indicator
				// to whether the min length should be increased.
				break
			}
		}
	}

	return idents, nil
}

func (s *Server) Get(def *Def, idents []*IdentAlias) ([]*IdentAlias, error) {
	conn := s.Pool.Get()
	defer conn.Close()

	for _, ia := range idents {
		lookupKey := mk(keyPrefix, def.ID, ia.Ident)

		// Check if the key already exists. If so, just return it.
		alias, err := redis.String(conn.Do("GET", lookupKey))

		// Exists.
		if err == nil {
			ia.Alias = alias
			ia.Status = StatusExists
			continue
		}

		if err != nil && err != redis.ErrNil {
			return nil, err
		}

		ia.Status = StatusMissing
	}

	return idents, nil
}

// Put explicitly sets a set of IDs with an alias.
func (s *Server) Put(def *Def, idents []*IdentAlias) error {
	conn := s.Pool.Get()
	defer conn.Close()

	for _, ia := range idents {
		if ia.Ident == "" {
			continue
		}

		if ia.Alias == "" {
			return errors.New("empty alias")
		}

		// key to alias
		lookupKey := mk(keyPrefix, def.ID, ia.Ident)
		// alias entry for existence check.
		checkKey := mk(aliasPrefix, def.ID, ia.Alias)

		_, err := conn.Do("MSET", lookupKey, ia.Alias, checkKey, true)
		if err != nil {
			return err
		}
	}

	s.Log.Printf("put %d keys", len(idents))

	return nil
}

func (s *Server) Del(def *Def, idents []string) error {
	conn := s.Pool.Get()
	defer conn.Close()

	var (
		removedCount  int
		skippedCount  int
		conflictCount int
		internalCount int
	)

	for _, ident := range idents {
		lookupKey := mk(keyPrefix, def.ID, ident)

		// Get the corresponding alias.
		alias, err := redis.String(conn.Do("GET", lookupKey))
		if err == redis.ErrNil {
			skippedCount++
			continue
		}

		if err != nil {
			return err
		}

		removedCount++

		checkKey := mk(aliasPrefix, def.ID, alias)

		n, err := redis.Int64(conn.Do("DEL", lookupKey, checkKey))
		if err != nil {
			return err
		}

		internalCount += int(n)
	}

	s.Log.Printf("%d removed", removedCount)
	s.Log.Printf("%d skipped", skippedCount)
	s.Log.Printf("%d conflicts", conflictCount)
	s.Log.Printf("%d internal", internalCount)

	return nil
}
