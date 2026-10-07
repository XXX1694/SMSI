package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/actor"
	"github.com/socialos/backend/internal/domain/errs"
)

const maxJSONBody = 1 << 20

// decode parses a JSON body (≤1 MiB). Empty bodies decode to the zero value.
func decode(r *http.Request, v any) error {
	if ct := r.Header.Get("Content-Type"); r.ContentLength != 0 && ct != "" && !strings.HasPrefix(ct, "application/json") {
		return errs.Validationf("Content-Type must be application/json")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody+1))
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		switch {
		case errors.As(err, &typ):
			return errs.Validationf("invalid type for field %q", typ.Field).WithField(typ.Field, "invalid type")
		case errors.As(err, &syn):
			return errs.Validationf("malformed JSON")
		default:
			return errs.Validationf("invalid request body")
		}
	}
	return nil
}

// pathID parses a UUID path parameter; malformed ids are NOT_FOUND.
func pathID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, errs.NotFoundf("resource")
	}
	return id, nil
}

// pageParams parses ?limit=&cursor=.
func pageParams(r *http.Request) (port.Page, error) {
	limit := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return port.Page{}, errs.Validationf("limit must be a positive integer").WithField("limit", "invalid")
		}
		limit = n
	}
	return port.NewPage(limit, r.URL.Query().Get("cursor"))
}

// queryTime parses an optional RFC 3339 query parameter.
func queryTime(r *http.Request, name string) (*time.Time, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, errs.Validationf("%s must be RFC 3339", name).WithField(name, "invalid time")
	}
	t = t.UTC()
	return &t, nil
}

// actorOf returns the request actor (RequireAuth guarantees presence).
func actorOf(r *http.Request) actor.Actor {
	a, _ := actor.From(r.Context())
	return a
}

// uuids parses a list of ids from strings.
func uuids(field string, in []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(in))
	for _, s := range in {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, errs.Validationf("%s contains an invalid id", field).WithField(field, "invalid id")
		}
		out = append(out, id)
	}
	return out, nil
}
