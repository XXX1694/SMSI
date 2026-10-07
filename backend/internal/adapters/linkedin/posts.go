package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"unicode/utf8"

	"github.com/socialos/backend/internal/adapters/provider"
)

type postBody struct {
	Author                    string         `json:"author"`
	Commentary                string         `json:"commentary"`
	Visibility                string         `json:"visibility"`
	Distribution              distribution   `json:"distribution"`
	Content                   map[string]any `json:"content,omitempty"`
	LifecycleState            string         `json:"lifecycleState"`
	IsReshareDisabledByAuthor bool           `json:"isReshareDisabledByAuthor"`
}

type distribution struct {
	FeedDistribution               string   `json:"feedDistribution"`
	TargetEntities                 []string `json:"targetEntities"`
	ThirdPartyDistributionChannels []string `json:"thirdPartyDistributionChannels"`
}

func permanent(code, msg string) *provider.Error {
	return &provider.Error{Kind: provider.KindPermanent, Provider: Name, Code: code, Message: msg}
}

// Publish creates a member post with optional images.
func (a *Adapter) Publish(ctx context.Context, req provider.PublishRequest) (provider.PublishResult, error) {
	if utf8.RuneCountInString(req.Text) > MaxTextLength {
		return provider.PublishResult{}, permanent("TEXT_TOO_LONG", fmt.Sprintf("LinkedIn posts are limited to %d characters", MaxTextLength))
	}
	if len(req.Media) > MaxImages {
		return provider.PublishResult{}, permanent("TOO_MANY_MEDIA", "too many images for LinkedIn")
	}
	author := personURN(req.Account.ProviderAccountID)
	var imageURNs []string
	for _, m := range req.Media {
		if m.Kind != "image" {
			return provider.PublishResult{}, &provider.Error{Kind: provider.KindUnsupported, Provider: Name, Code: "VIDEO_UNSUPPORTED", Message: "LinkedIn video is not supported in this release"}
		}
		urn, err := a.uploadImage(ctx, req.AccessToken, author, m)
		if err != nil {
			return provider.PublishResult{}, err
		}
		imageURNs = append(imageURNs, urn)
	}
	body := postBody{
		Author:         author,
		Commentary:     FormatCommentary(req.Text),
		Visibility:     "PUBLIC",
		Distribution:   distribution{FeedDistribution: "MAIN_FEED", TargetEntities: []string{}, ThirdPartyDistributionChannels: []string{}},
		Content:        mediaContent(imageURNs),
		LifecycleState: "PUBLISHED",
	}
	return a.createPost(ctx, req.AccessToken, body)
}

func mediaContent(urns []string) map[string]any {
	switch len(urns) {
	case 0:
		return nil
	case 1:
		return map[string]any{"media": map[string]any{"id": urns[0]}}
	default:
		imgs := make([]map[string]any, len(urns))
		for i, u := range urns {
			imgs[i] = map[string]any{"id": u}
		}
		return map[string]any{"multiImage": map[string]any{"images": imgs}}
	}
}

func (a *Adapter) createPost(ctx context.Context, token string, body postBody) (provider.PublishResult, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return provider.PublishResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.APIBaseURL+"/rest/posts", bytes.NewReader(buf))
	if err != nil {
		return provider.PublishResult{}, err
	}
	a.setRESTHeaders(req, token)
	resp, err := a.http.Do(req)
	if err != nil {
		return provider.PublishResult{}, err
	}
	defer drain(resp)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return provider.PublishResult{}, errorFromResponse(resp)
	}
	urn := resp.Header.Get("x-restli-id")
	if urn == "" {
		urn = resp.Header.Get("X-LinkedIn-Id")
	}
	if urn == "" {
		// Post was created but we cannot identify it: report unknown, never retry blindly.
		return provider.PublishResult{}, &provider.Error{Kind: provider.KindUnknown, Provider: Name, Code: "MISSING_POST_ID", Message: "LinkedIn did not return the post id"}
	}
	return provider.PublishResult{
		ExternalID: urn,
		URL:        "https://www.linkedin.com/feed/update/" + urn + "/",
		Metadata:   map[string]any{"linkedin_version": a.cfg.APIVersion},
	}, nil
}

// Delete removes a post; an already-deleted post counts as success.
func (a *Adapter) Delete(ctx context.Context, d provider.DeleteRequest) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, a.cfg.APIBaseURL+"/rest/posts/"+url.PathEscape(d.ExternalID), nil)
	if err != nil {
		return err
	}
	a.setRESTHeaders(req, d.AccessToken)
	req.Header.Set("X-RestLi-Method", "DELETE")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return errorFromResponse(resp)
}
