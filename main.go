package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

var buildVersion string

func main() {
	var (
		redisAddr string
		redisDB   int
		redisPass string
		redisTLS  bool

		httpAddr    string
		httpTLSKey  string
		httpTLSCert string

		showVersion bool
	)

	flag.StringVar(&redisAddr, "redis", "127.0.0.1:6379", "Redis address.")
	flag.IntVar(&redisDB, "redis.db", 0, "Redis database.")
	flag.StringVar(&redisPass, "redis.pass", "", "Redis password.")
	flag.BoolVar(&redisTLS, "redis.tls", false, "Redis TLS connection.")

	flag.StringVar(&httpAddr, "http", "127.0.0.1:8080", "HTTP bind address.")
	flag.StringVar(&httpTLSKey, "http.tls.key", "", "TLS key file.")
	flag.StringVar(&httpTLSCert, "http.tls.cert", "", "TLS certificate file.")

	flag.BoolVar(&showVersion, "version", false, "Print the program version")

	flag.Parse()

	if showVersion {
		fmt.Println(buildVersion)
		return
	}

	var s Server

	s.RedisAddr = redisAddr
	s.RedisDB = redisDB
	s.RedisPass = redisPass
	s.RedisTLS = redisTLS
	s.Init()

	defer s.Close()

	mux := httprouter.New()

	mux.GET("/defs", makeGetDefsHandler(&s))
	mux.POST("/defs", makeCreateDefHandler(&s))

	mux.GET("/defs/:name", makeGetDefHandler(&s))
	mux.PUT("/defs/:name", makeUpdateDefHandler(&s))
	mux.DELETE("/defs/:name", makeDeleteDefHandler(&s))

	mux.POST("/keys/:name", makeGenHandler(&s))
	mux.PUT("/keys/:name", makePutHandler(&s))
	mux.DELETE("/keys/:name", makeDeleteHandler(&s))

	log.Printf("HTTP listening on %s", httpAddr)
	if httpTLSKey != "" {
		log.Fatal(http.ListenAndServeTLS(httpAddr, httpTLSCert, httpTLSKey, mux))
	} else {
		log.Fatal(http.ListenAndServe(httpAddr, mux))
	}
}
