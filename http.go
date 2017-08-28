package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

const applicationJson = "application/json"

func makeCreateDefHandler(s *Server) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		defer r.Body.Close()

		// New generator definition with defaults.
		def := NewDef()

		if err := json.NewDecoder(r.Body).Decode(def); err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprintf(w, err.Error())
			return
		}

		err := s.CreateDef(def)

		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

func makeUpdateDefHandler(s *Server) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		name := p.ByName("name")

		def, err := s.GetDef(name)
		if err == ErrNoDef {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		id := def.ID

		defer r.Body.Close()

		if err := json.NewDecoder(r.Body).Decode(def); err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprintf(w, err.Error())
			return
		}

		def.ID = id

		if err = s.UpdateDef(name, def); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func makeGetDefsHandler(s *Server) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		defs, err := s.GetDefs()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		w.Header().Set("content-type", applicationJson)

		if err := json.NewEncoder(w).Encode(defs); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, err.Error())
			return
		}
	}
}

func makeDeleteDefHandler(s *Server) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		name := p.ByName("name")

		err := s.DelDef(name)
		if err == ErrNoDef {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func makeGetDefHandler(s *Server) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		name := p.ByName("name")

		def, err := s.GetDef(name)
		if err == ErrNoDef {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		b, err := json.Marshal(def)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		w.Header().Set("content-type", applicationJson)
		w.Write(b)
	}
}

func parseGenBody(mediaType string, r io.Reader) ([]*IdentAlias, error) {
	// Decode request body containing the aliases.
	var (
		err    error
		idents []*IdentAlias
	)

	switch mediaType {
	case applicationJson:
		var a []string
		err = json.NewDecoder(r).Decode(&a)

		idents = make([]*IdentAlias, len(a))

		for i, ident := range a {
			idents[i] = &IdentAlias{
				Ident: ident,
			}
		}

	default:
		sc := bufio.NewScanner(r)

		for sc.Scan() {
			idents = append(idents, &IdentAlias{
				Ident: sc.Text(),
			})
		}

		err = sc.Err()
	}

	if err != nil {
		return nil, err
	}

	return idents, nil
}

func makeGenHandler(s *Server) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		name := p.ByName("name")
		readOnly := r.URL.Query().Get("ro") != ""

		def, err := s.GetDef(name)
		if err == ErrNoDef {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Something else wrong.
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("content-type"))

		idents, err := parseGenBody(mediaType, r.Body)

		r.Body.Close()

		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, err.Error())
			return
		}

		if readOnly {
			idents, err = s.Get(def, idents)
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, err.Error())
				return
			}

			switch mediaType {
			case applicationJson:
				w.Header().Set("content-type", applicationJson)
				json.NewEncoder(w).Encode(idents)

			default:
				for _, ia := range idents {
					switch ia.Status {
					case StatusExists:
						fmt.Fprintln(w, "1", ia.Alias)
					case StatusMissing:
						fmt.Fprintln(w, "0")
					}
				}
			}

			return
		}

		idents, err = s.Gen(def, idents)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		switch mediaType {
		case applicationJson:
			w.Header().Set("content-type", applicationJson)
			json.NewEncoder(w).Encode(idents)

		default:
			for _, ia := range idents {
				switch ia.Status {
				case StatusExists:
					fmt.Fprintln(w, "0", ia.Alias)
				case StatusCreated:
					fmt.Fprintln(w, "1", ia.Alias)
				}
			}
		}
	}
}

func parsePutBody(mediaType string, r io.Reader) ([]*IdentAlias, error) {
	var (
		err    error
		idents []*IdentAlias
	)

	switch mediaType {
	case applicationJson:
		err = json.NewDecoder(r).Decode(&idents)

	default:
		sr := bufio.NewScanner(r)

		for sr.Scan() {
			line := sr.Text()
			toks := splitRegex.Split(line, 2)

			if len(toks) != 2 {
				return nil, errors.New("delimiter should match [\\s\\t,]+")
			}

			idents = append(idents, &IdentAlias{
				Ident: string(toks[0]),
				Alias: string(toks[1]),
			})
		}

		err = sr.Err()
	}

	if err != nil {
		return nil, err
	}

	return idents, nil
}

func makePutHandler(s *Server) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		name := p.ByName("name")

		def, err := s.GetDef(name)
		if err == ErrNoDef {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Something else wrong.
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("content-type"))

		idents, err := parsePutBody(mediaType, r.Body)

		r.Body.Close()

		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, err.Error())
			return
		}

		if err := s.Put(def, idents); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func parseDeleteBody(r io.Reader) ([]string, error) {
	var a []string

	sc := bufio.NewScanner(r)

	for sc.Scan() {
		a = append(a, sc.Text())
	}

	return a, sc.Err()
}

func makeDeleteHandler(s *Server) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		name := p.ByName("name")

		def, err := s.GetDef(name)
		if err == ErrNoDef {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Something else wrong.
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("content-type"))

		var idents []string

		switch mediaType {
		case applicationJson:
			err = json.NewDecoder(r.Body).Decode(&idents)

		default:
			idents, err = parseDeleteBody(r.Body)
		}

		r.Body.Close()

		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, err.Error())
			return
		}

		if err := s.Del(def, idents); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
