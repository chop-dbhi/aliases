package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	uuid "github.com/satori/go.uuid"
)

var (
	DefaultCharsMinlen = 8
	DefaultCharsValid  = "abcdefghijklmnopqrstuvwzyz0123456789"

	keyPrefix   = "k:"
	aliasPrefix = "a:"

	buildVersion string
)

func main() {
	var (
		redisAddr string
		redisDB   int
		redisPass string

		httpAddr    string
		httpTlsKey  string
		httpTlsCert string

		aliasGen AliasGen

		showVersion bool
	)

	flag.StringVar(&redisAddr, "redis", "127.0.0.1:6379", "Redis address.")
	flag.IntVar(&redisDB, "redis.db", 0, "Redis database.")
	flag.StringVar(&redisPass, "redis.pass", "", "Redis password.")

	flag.StringVar(&httpAddr, "http", "127.0.0.1:8080", "HTTP bind address.")
	flag.StringVar(&httpTlsKey, "http.tls.key", "", "TLS key file.")
	flag.StringVar(&httpTlsCert, "http.tls.cert", "", "TLS certificate file.")

	flag.StringVar(&aliasGen.Prefix, "prefix", "", "Prefix of the generated alias.")
	flag.StringVar(&aliasGen.Type, "type", "chars", "Type of alias generation. Options are 'chars' and 'uuid'.")
	flag.IntVar(&aliasGen.Minlen, "chars.minlen", DefaultCharsMinlen, "Minimum length of the alias, ignoring the prefix.")
	flag.StringVar(&aliasGen.Chars, "chars.valid", DefaultCharsValid, "Valid characters to use for alias.")

	flag.BoolVar(&showVersion, "version", false, "Print the program version")

	flag.Parse()

	if showVersion {
		fmt.Println(buildVersion)
		return
	}

	// Create a pool of Redis connections.
	pool := redis.Pool{
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", redisAddr)
			if err != nil {
				return nil, err
			}

			// Password to authenticate.
			if redisPass != "" {
				if _, err := conn.Do("AUTH", redisPass); err != nil {
					return nil, err
				}
			}

			if _, err := conn.Do("SELECT", redisDB); err != nil {
				return nil, err
			}

			return conn, nil
		},
		// TODO: make configuration options.
		IdleTimeout: 5 * time.Minute,
		MaxIdle:     3,
	}

	defer pool.Close()

	http.HandleFunc("/", makeRequestHandler(&pool, &aliasGen))

	log.Printf("HTTP listening on %s", httpAddr)
	if httpTlsKey != "" {
		log.Fatal(http.ListenAndServeTLS(httpAddr, httpTlsCert, httpTlsKey, nil))
	} else {
		log.Fatal(http.ListenAndServe(httpAddr, nil))
	}
}

func makeRequestHandler(pool *redis.Pool, aliasGen *AliasGen) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		conn := pool.Get()
		defer conn.Close()

		// Line delimited identifiers.
		s := bufio.NewScanner(r.Body)
		defer r.Body.Close()

		for s.Scan() {
			key := s.Text()

			// Check if the key already exists. If so, just return it.
			alias, err := redis.String(conn.Do("GET", keyPrefix+key))
			// TODO: retry for certain errors.
			if err != nil && err != redis.ErrNil {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, err.Error())
				return
			}

			// Exists. Write it out and go to the next one.
			if alias != "" {
				fmt.Fprintln(w, alias)
				continue
			}

			// Generate one.
			var attempt int
			maxAttempts := 100

			for {
				if attempt == maxAttempts {
					// TODO: in theory, this should not occur assuming the min length
					// has been increased proportionally to the aliases that have been
					// issued.
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprint(w, "reached max attempts")
					return
				}

				attempt++

				// Generate new key.
				alias = aliasGen.New()

				// Check if it exists, otherwise set it.
				ok, err := redis.Bool(conn.Do("EXISTS", aliasPrefix+alias))
				if err != nil && err != redis.ErrNil {
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprint(w, err.Error())
					return
				}

				if !ok {
					pKey := keyPrefix + key
					pAlias := aliasPrefix + alias

					// Watch these keys for concurrent sets.
					conn.Send("WATCH", pKey, pAlias)

					// Start transaction.
					conn.Send("MULTI")

					conn.Send("SET", pKey, alias)
					conn.Send("SET", pAlias, true)

					// Finish transaction.
					reply, err := conn.Do("EXEC")
					if err != nil && err != redis.ErrNil {
						w.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprint(w, err.Error())
					}

					// A nil reply implies a conflict with one of the keys. Try again.
					// See: https://redis.io/commands/exec
					if reply == nil {
						continue
					}

					// TODO: add metric for number of attempts. this is an indicator
					// to whether the min length should be increased.
					break
				}
			}

			// Write alias to response.
			fmt.Fprintln(w, alias)
		}

		if err := s.Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}
	}
}

// AliasGen is a key generator.
type AliasGen struct {
	Type   string
	Prefix string
	Minlen int
	Chars  string
}

// New generates a new key.
func (g *AliasGen) New() string {
	var alias string

	switch strings.ToLower(g.Type) {
	case "uuid":
		alias = uuid.NewV4().String()

	default:
		key := make([]byte, g.Minlen)
		charlen := len(g.Chars)

		for i := range key {
			c := rand.Intn(charlen)
			key[i] = g.Chars[c]
		}

		alias = string(key)
	}

	if g.Prefix == "" {
		return alias
	}

	return fmt.Sprintf("%s%s", g.Prefix, alias)
}
