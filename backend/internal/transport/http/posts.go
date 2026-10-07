package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/application/posts"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/post"
	"github.com/socialos/backend/internal/transport/httpx"
)

type targetReq struct {
	SocialAccountID string  `json:"social_account_id"`
	Content         *string `json:"content"`
}

type createPostReq struct {
	Title            string      `json:"title"`
	Content          string      `json:"content"`
	SocialAccountIDs []string    `json:"social_account_ids"`
	MediaIDs         []string    `json:"media_ids"`
	Targets          []targetReq `json:"targets"`
	ScheduledAt      *time.Time  `json:"scheduled_at"`
	Schedule         bool        `json:"schedule"`
}

type updatePostReq struct {
	Title            *string      `json:"title"`
	Content          *string      `json:"content"`
	SocialAccountIDs *[]string    `json:"social_account_ids"`
	MediaIDs         *[]string    `json:"media_ids"`
	Targets          *[]targetReq `json:"targets"`
	ScheduledAt      *time.Time   `json:"scheduled_at"`
}

func parseTargets(in []targetReq) ([]posts.TargetInput, error) {
	out := make([]posts.TargetInput, 0, len(in))
	for _, t := range in {
		id, err := uuid.Parse(t.SocialAccountID)
		if err != nil {
			return nil, errs.Validationf("targets contains an invalid social_account_id").WithField("targets", "invalid id")
		}
		out = append(out, posts.TargetInput{SocialAccountID: id, Content: t.Content})
	}
	return out, nil
}

func (req createPostReq) toInput() (posts.CreateInput, error) {
	accs, err := uuids("social_account_ids", req.SocialAccountIDs)
	if err != nil {
		return posts.CreateInput{}, err
	}
	mids, err := uuids("media_ids", req.MediaIDs)
	if err != nil {
		return posts.CreateInput{}, err
	}
	targets, err := parseTargets(req.Targets)
	if err != nil {
		return posts.CreateInput{}, err
	}
	return posts.CreateInput{Title: req.Title, Content: req.Content, SocialAccountIDs: accs, MediaIDs: mids,
		Targets: targets, ScheduledAt: req.ScheduledAt, Schedule: req.Schedule}, nil
}

func (req updatePostReq) toInput() (posts.UpdateInput, error) {
	in := posts.UpdateInput{Title: req.Title, Content: req.Content, ScheduledAt: req.ScheduledAt}
	if req.SocialAccountIDs != nil {
		ids, err := uuids("social_account_ids", *req.SocialAccountIDs)
		if err != nil {
			return in, err
		}
		in.SocialAccountIDs = &ids
	}
	if req.MediaIDs != nil {
		ids, err := uuids("media_ids", *req.MediaIDs)
		if err != nil {
			return in, err
		}
		in.MediaIDs = &ids
	}
	if req.Targets != nil {
		ts, err := parseTargets(*req.Targets)
		if err != nil {
			return in, err
		}
		in.Targets = &ts
	}
	return in, nil
}

func (a *API) createPost(w http.ResponseWriter, r *http.Request) {
	var req createPostReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	in, err := req.toInput()
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := a.svc.Posts.Create(r.Context(), actorOf(r), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toPost(p))
}

func (a *API) listPosts(w http.ResponseWriter, r *http.Request) {
	pg, err := pageParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	f := posts.ListFilter{Status: post.Status(r.URL.Query().Get("status"))}
	if f.From, err = queryTime(r, "from"); err == nil {
		f.To, err = queryTime(r, "to")
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := a.svc.Posts.List(r.Context(), actorOf(r), f, pg)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, newPage(toPosts(res.Items), res.NextCursor))
}

func (a *API) getPost(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	act := actorOf(r)
	d, err := a.svc.Posts.Get(r.Context(), act, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	media, err := a.svc.Media.ByIDs(r.Context(), act, d.MediaIDs)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toPostDetail(d.Post, d.Attempts, media))
}

func (a *API) postStatus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := a.svc.Posts.Status(r.Context(), actorOf(r), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	d := toPost(p)
	httpx.JSON(w, http.StatusOK, map[string]any{"id": d.ID, "status": d.Status, "scheduled_at": d.ScheduledAt,
		"published_at": d.PublishedAt, "targets": d.Targets})
}

func (a *API) updatePost(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req updatePostReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	in, err := req.toInput()
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := a.svc.Posts.Update(r.Context(), actorOf(r), id, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toPost(p))
}

func (a *API) deletePost(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err == nil {
		err = a.svc.Posts.Delete(r.Context(), actorOf(r), id)
	}
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type scheduleReq struct {
	ScheduledAt *time.Time `json:"scheduled_at"`
}

type retryReq struct {
	ScheduledAt        *time.Time `json:"scheduled_at"`
	IncludeNeedsReview bool       `json:"include_needs_review"`
}

func (a *API) schedulePost(w http.ResponseWriter, r *http.Request) {
	var req scheduleReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.ScheduledAt == nil {
		httpx.Error(w, r, errs.Validationf("scheduled_at is required").WithField("scheduled_at", "required"))
		return
	}
	a.postAction(w, r, http.StatusOK, func(id uuid.UUID) (*post.Post, error) {
		return a.svc.Posts.Schedule(r.Context(), actorOf(r), id, req.ScheduledAt.UTC())
	})
}

func (a *API) retryPost(w http.ResponseWriter, r *http.Request) {
	var req retryReq
	if err := decode(r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	a.postAction(w, r, http.StatusAccepted, func(id uuid.UUID) (*post.Post, error) {
		return a.svc.Posts.Retry(r.Context(), actorOf(r), id, posts.RetryInput{ScheduledAt: req.ScheduledAt, IncludeNeedsReview: req.IncludeNeedsReview})
	})
}

func (a *API) publishPost(w http.ResponseWriter, r *http.Request) {
	a.postAction(w, r, http.StatusAccepted, func(id uuid.UUID) (*post.Post, error) {
		return a.svc.Posts.PublishNow(r.Context(), actorOf(r), id)
	})
}

func (a *API) cancelPost(w http.ResponseWriter, r *http.Request) {
	a.postAction(w, r, http.StatusOK, func(id uuid.UUID) (*post.Post, error) {
		return a.svc.Posts.Cancel(r.Context(), actorOf(r), id)
	})
}

func (a *API) unschedulePost(w http.ResponseWriter, r *http.Request) {
	a.postAction(w, r, http.StatusOK, func(id uuid.UUID) (*post.Post, error) {
		return a.svc.Posts.Unschedule(r.Context(), actorOf(r), id)
	})
}

func (a *API) postAction(w http.ResponseWriter, r *http.Request, status int, fn func(uuid.UUID) (*post.Post, error)) {
	id, err := pathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := fn(id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, status, toPost(p))
}
