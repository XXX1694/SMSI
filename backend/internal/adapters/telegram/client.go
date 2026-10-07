package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/socialos/backend/internal/adapters/provider"
)

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (a *Adapter) methodURL(method string) string { return a.base + "/bot" + a.token + "/" + method }

// redact removes the bot token from any error text (URLs embed the token).
func (a *Adapter) redact(err error) error {
	if err == nil || a.token == "" {
		return err
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return &url.Error{Op: ue.Op, URL: strings.ReplaceAll(ue.URL, a.token, "<redacted>"), Err: ue.Err}
	}
	if strings.Contains(err.Error(), a.token) {
		return errors.New(strings.ReplaceAll(err.Error(), a.token, "<redacted>"))
	}
	return err
}

// callJSON invokes a Bot API method with a JSON body and decodes result into out.
func (a *Adapter) callJSON(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.methodURL(method), bytes.NewReader(body))
	if err != nil {
		return a.redact(err)
	}
	req.Header.Set("Content-Type", "application/json")
	return a.do(req, out)
}

func (a *Adapter) do(req *http.Request, out any) error {
	if a.token == "" {
		return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "BOT_NOT_CONFIGURED", Message: "TELEGRAM_BOT_TOKEN is not configured"}
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return a.redact(err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}()
	var ar apiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&ar); err != nil {
		if resp.StatusCode >= 500 {
			return provider.FromHTTPStatus(Name, resp.StatusCode, "Telegram unavailable", 0)
		}
		return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "BAD_RESPONSE", Message: "invalid Telegram response", HTTPStatus: resp.StatusCode}
	}
	if !ar.OK {
		return classify(ar, resp.StatusCode)
	}
	if out != nil {
		if err := json.Unmarshal(ar.Result, out); err != nil {
			return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "BAD_RESULT", Message: "unexpected Telegram result"}
		}
	}
	return nil
}

func classify(ar apiResponse, httpStatus int) *provider.Error {
	code := ar.ErrorCode
	if code == 0 {
		code = httpStatus
	}
	msg := "Telegram: " + ar.Description
	retry := time.Duration(ar.Parameters.RetryAfter) * time.Second
	switch code {
	case 401, 404:
		// 401/404 on the bot endpoint means the bot token itself is invalid: operator config, not the user's account.
		return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: "BOT_UNAUTHORIZED", Message: "Telegram bot token is invalid", HTTPStatus: code}
	case 403:
		return &provider.Error{Kind: provider.KindAuth, Provider: Name, Code: "BOT_FORBIDDEN", Message: msg, HTTPStatus: code}
	default:
		e := provider.FromHTTPStatus(Name, code, msg, retry)
		e.Code = fmt.Sprintf("TG_%d", code)
		return e
	}
}
