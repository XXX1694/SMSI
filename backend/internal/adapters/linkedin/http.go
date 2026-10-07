package linkedin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

const maxBody = 1 << 20

func limited(resp *http.Response) io.Reader { return io.LimitReader(resp.Body, maxBody) }

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody))
	_ = resp.Body.Close()
}

type apiError struct {
	Message          string `json:"message"`
	ServiceErrorCode int    `json:"serviceErrorCode"`
	Code             string `json:"code"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// errorFromResponse converts a non-2xx response into a classified error.
// Only LinkedIn's error message is kept; request headers (tokens) never are.
func errorFromResponse(resp *http.Response) *provider.Error {
	var ae apiError
	_ = json.NewDecoder(limited(resp)).Decode(&ae)
	msg := ae.Message
	if msg == "" {
		msg = ae.ErrorDescription
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	pe := provider.FromHTTPStatus(Name, resp.StatusCode, "LinkedIn: "+msg, retryAfter(resp))
	switch {
	case ae.Error != "":
		pe.Code = ae.Error
	case ae.Code != "":
		pe.Code = ae.Code
	}
	return pe
}

func retryAfter(resp *http.Response) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 0
}

func (a *Adapter) setRESTHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("LinkedIn-Version", a.cfg.APIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	req.Header.Set("Content-Type", "application/json")
}
