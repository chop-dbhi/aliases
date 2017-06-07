package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

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

		w.Header().Set("content-type", "application/json")

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

		w.Header().Set("content-type", "application/json")
		w.Write(b)
	}
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

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, r.Body); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err.Error())
			return
		}

		r.Body.Close()

		if readOnly {
			if err := s.Get(def, &buf, w); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, err.Error())
				return
			}

			return
		}

		if err := s.Gen(def, &buf, w); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}
	}
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

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, r.Body); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err.Error())
			return
		}

		r.Body.Close()

		if err := s.Put(def, &buf); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}
	}
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

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, r.Body); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err.Error())
			return
		}

		r.Body.Close()

		if err := s.Del(def, &buf); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, err.Error())
			return
		}
	}
}
