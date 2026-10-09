package bluesky

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/socialos/backend/internal/adapters/provider"
)

type blob map[string]any

type uploadResponse struct {
	Blob blob `json:"blob"`
}

type recordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

// targetFor builds the session target of an account from non-secret metadata.
func (a *Adapter) targetFor(acc provider.AccountRef, password string) (target, error) {
	did := acc.ProviderAccountID
	if did == "" {
		return target{}, permanent("BAD_ACCOUNT", "The Bluesky account has no DID: reconnect it")
	}
	pds, _ := acc.Metadata["pds"].(string)
	base, _, err := a.baseFor(pds)
	if err != nil {
		return target{}, err
	}
	return target{key: acc.ID, base: base, identifier: did, password: password}, nil
}

func (a *Adapter) urlFor(sess *session, acc provider.AccountRef, rkey string) string {
	handle := sess.handle
	if handle == "" {
		handle = acc.Username
	}
	return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", handle, rkey)
}

// Publish writes an app.bsky.feed.post record with a deterministic rkey.
func (a *Adapter) Publish(ctx context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	if err := checkContent(req); err != nil {
		return provider.PublishResult{}, err
	}
	t, err := a.targetFor(req.Account, req.AccessToken)
	if err != nil {
		return provider.PublishResult{}, err
	}
	images, err := readImages(ctx, req.Media)
	if err != nil {
		return provider.PublishResult{}, err
	}
	rkey := rkeyFor(req.IdempotencyKey, a.cfg.Now())
	var res provider.PublishResult
	err = a.withSession(ctx, t, func(s *session) error {
		embed, err := a.uploadAll(ctx, t, s, images)
		if err != nil {
			return err
		}
		rec := map[string]any{"$type": postType, "text": req.Text,
			"createdAt": a.cfg.Now().UTC().Format("2006-01-02T15:04:05.000Z")}
		if fs := buildFacets(req.Text); len(fs) > 0 {
			rec["facets"] = fs
		}
		if embed != nil {
			rec["embed"] = embed
		}
		var out recordResponse
		err = a.do(ctx, call{base: t.base, method: http.MethodPost, nsid: "com.atproto.repo.createRecord", bearer: s.accessJwt,
			body: map[string]any{"repo": s.did, "collection": postType, "rkey": rkey, "record": rec}}, &out)
		if err != nil && !isAuth(err) && a.exists(ctx, t, s, rkey) {
			// A repeated attempt hit its own record: the earlier one succeeded.
			err = nil
		}
		if err != nil {
			return err
		}
		res = provider.PublishResult{ExternalID: rkey, URL: a.urlFor(s, req.Account, rkey), Metadata: map[string]any{"uri": out.URI, "cid": out.CID}}
		return nil
	})
	return res, err
}

func (a *Adapter) exists(ctx context.Context, t target, s *session, rkey string) bool {
	_, ok, err := a.getRecord(ctx, t, s, rkey)
	return err == nil && ok
}

func (a *Adapter) getRecord(ctx context.Context, t target, s *session, rkey string) (recordResponse, bool, error) {
	var out recordResponse
	err := a.do(ctx, call{base: t.base, method: http.MethodGet, nsid: "com.atproto.repo.getRecord", bearer: s.accessJwt,
		query: url.Values{"repo": {s.did}, "collection": {postType}, "rkey": {rkey}}}, &out)
	var pe *provider.Error
	if errors.As(err, &pe) && pe.Code == "RecordNotFound" {
		return out, false, nil
	}
	return out, err == nil, err
}

// Lookup resolves an unknown outcome: the rkey derives from the idempotency key.
func (a *Adapter) Lookup(ctx context.Context, req provider.PublishRequest) (provider.PublishResult, bool, error) {
	if req.IdempotencyKey == "" {
		return provider.PublishResult{}, false, nil
	}
	t, err := a.targetFor(req.Account, req.AccessToken)
	if err != nil {
		return provider.PublishResult{}, false, err
	}
	rkey := rkeyFor(req.IdempotencyKey, a.cfg.Now())
	var res provider.PublishResult
	found := false
	err = a.withSession(ctx, t, func(s *session) error {
		out, ok, err := a.getRecord(ctx, t, s, rkey)
		if err != nil || !ok {
			return err
		}
		found = true
		res = provider.PublishResult{ExternalID: rkey, URL: a.urlFor(s, req.Account, rkey), Metadata: map[string]any{"uri": out.URI, "cid": out.CID}}
		return nil
	})
	return res, found, err
}

// Delete removes the post record. Deleting a missing record succeeds on the PDS.
func (a *Adapter) Delete(ctx context.Context, req provider.DeleteRequest) error {
	if req.ExternalID == "" {
		return permanent("BAD_REQUEST", "no post id to delete")
	}
	t, err := a.targetFor(req.Account, req.AccessToken)
	if err != nil {
		return err
	}
	return a.withSession(ctx, t, func(s *session) error {
		return a.do(ctx, call{base: t.base, method: http.MethodPost, nsid: "com.atproto.repo.deleteRecord", bearer: s.accessJwt,
			body: map[string]any{"repo": s.did, "collection": postType, "rkey": req.ExternalID}}, nil)
	})
}

func checkContent(req provider.PublishRequest) error {
	if n := graphemeCount(req.Text); n > MaxGraphemes {
		return permanent("TEXT_TOO_LONG", fmt.Sprintf("Bluesky posts are limited to %d characters; this one has %d", MaxGraphemes, n))
	}
	if len(req.Text) > maxTextBytes {
		return permanent("TEXT_TOO_LONG", "The post text is too large for Bluesky")
	}
	if req.Text == "" && len(req.Media) == 0 {
		return permanent("EMPTY_POST", "A Bluesky post needs text or an image")
	}
	if len(req.Media) > MaxImages {
		return permanent("TOO_MANY_IMAGES", fmt.Sprintf("Bluesky allows up to %d images", MaxImages))
	}
	return nil
}

type image struct {
	data []byte
	mime string
}

func readImages(ctx context.Context, media []provider.MediaFile) ([]image, error) {
	out := make([]image, 0, len(media))
	for _, m := range media {
		if m.Kind != "image" {
			return nil, &provider.Error{Kind: provider.KindUnsupported, Provider: Name, Code: "UNSUPPORTED_MEDIA", Message: "Bluesky posts support images only in Steerpost", Err: provider.ErrUnsupported}
		}
		if m.Size > MaxImageBytes {
			return nil, permanent("IMAGE_TOO_LARGE", "Bluesky images must be 2 MB or smaller")
		}
		rc, err := m.Open(ctx)
		if err != nil {
			return nil, &provider.Error{Kind: provider.KindRetryable, Provider: Name, Code: "MEDIA_READ", Message: "could not read media from storage", Err: err}
		}
		data, err := io.ReadAll(io.LimitReader(rc, MaxImageBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, &provider.Error{Kind: provider.KindRetryable, Provider: Name, Code: "MEDIA_READ", Message: "could not read media from storage", Err: err}
		}
		if len(data) > MaxImageBytes {
			return nil, permanent("IMAGE_TOO_LARGE", "Bluesky images must be 2 MB or smaller")
		}
		out = append(out, image{data: data, mime: m.MimeType})
	}
	return out, nil
}

// uploadAll uploads the blobs and returns the app.bsky.embed.images embed (nil if none).
func (a *Adapter) uploadAll(ctx context.Context, t target, s *session, images []image) (map[string]any, error) {
	if len(images) == 0 {
		return nil, nil
	}
	list := make([]map[string]any, 0, len(images))
	for _, im := range images {
		var up uploadResponse
		if err := a.do(ctx, call{base: t.base, method: http.MethodPost, nsid: "com.atproto.repo.uploadBlob", bearer: s.accessJwt,
			raw: im.data, rawType: im.mime}, &up); err != nil {
			return nil, err
		}
		list = append(list, map[string]any{"alt": "", "image": up.Blob})
	}
	return map[string]any{"$type": "app.bsky.embed.images", "images": list}, nil
}
