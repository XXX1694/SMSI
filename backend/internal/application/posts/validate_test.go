package posts

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/media"
	"github.com/socialos/backend/internal/domain/post"
)

func img() media.Media { return media.Media{ID: uuid.New(), Kind: media.KindImage} }
func vid() media.Media { return media.Media{ID: uuid.New(), Kind: media.KindVideo} }

var textOnly = provider.Capabilities{CanPublishText: true, MaxTextLength: 10, MaxMediaCount: 0}
var photoNet = provider.Capabilities{CanPublishImage: true, MaxTextLength: 50, MaxCaptionLength: 20, MaxMediaCount: 2}
var full = provider.Capabilities{CanPublishText: true, CanPublishImage: true, CanPublishVideo: true, MaxTextLength: 100, MaxMediaCount: 3}

func TestCheckContent(t *testing.T) {
	for name, tc := range map[string]struct {
		caps  provider.Capabilities
		text  string
		media []media.Media
		field string // expected failing field, "" = ok
	}{
		"text within limit":                {textOnly, "0123456789", nil, ""},
		"text over limit":                  {textOnly, "01234567890", nil, "content"},
		"limit counts characters":          {textOnly, "日本語日本語日本語日", nil, ""},
		"limit counts characters, over":    {textOnly, "日本語日本語日本語日本", nil, "content"},
		"empty":                            {full, "", nil, "content"},
		"whitespace only":                  {full, " \n\t ", nil, "content"},
		"media only is fine":               {full, "", []media.Media{img()}, ""},
		"image on a text-only network":     {textOnly, "hi", []media.Media{img()}, "media_ids"},
		"too many media":                   {full, "x", []media.Media{img(), img(), img(), img()}, "media_ids"},
		"exactly the media limit":          {full, "x", []media.Media{img(), img(), img()}, ""},
		"video unsupported":                {photoNet, "x", []media.Media{vid()}, "media_ids"},
		"video supported":                  {full, "x", []media.Media{vid()}, ""},
		"network requires media":           {photoNet, "caption", nil, "media_ids"},
		"caption limit applies with media": {photoNet, strings.Repeat("c", 21), []media.Media{img()}, "content"},
		"caption within limit":             {photoNet, strings.Repeat("c", 20), []media.Media{img()}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			err := CheckContent("Net", tc.caps, tc.text, tc.media)
			if tc.field == "" {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				return
			}
			e, ok := errs.As(err)
			if !ok || e.Code != errs.Validation || e.Fields[tc.field] == "" {
				t.Fatalf("want a validation error on %q, got %v", tc.field, err)
			}
			if !strings.Contains(e.Message, "Net") {
				t.Errorf("message should name the network: %q", e.Message)
			}
		})
	}
}

func TestMergeAccounts(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	over := "only for b"
	ids, overrides := mergeAccounts([]uuid.UUID{a, b, a}, []TargetInput{{SocialAccountID: b, Content: &over}, {SocialAccountID: c}})
	if len(ids) != 3 || ids[0] != a || ids[1] != b || ids[2] != c {
		t.Fatalf("order and de-duplication: %v", ids)
	}
	if len(overrides) != 1 || overrides[b] != over {
		t.Fatalf("overrides: %v", overrides)
	}
	if ids, ov := mergeAccounts(nil, nil); len(ids) != 0 || len(ov) != 0 {
		t.Fatal("empty input")
	}
	// An explicit empty override is kept (it is different from "no override").
	empty := ""
	if _, ov := mergeAccounts(nil, []TargetInput{{SocialAccountID: a, Content: &empty}}); len(ov) != 1 {
		t.Fatalf("empty override dropped: %v", ov)
	}
}

func TestValidateBasics(t *testing.T) {
	for name, tc := range map[string]struct {
		title, content string
		accounts, mids int
		field          string
	}{
		"ok":                {"t", "c", 1, 1, ""},
		"all empty is ok":   {"", "", 0, 0, ""},
		"title at limit":    {strings.Repeat("t", MaxTitleLen), "", 0, 0, ""},
		"title over":        {strings.Repeat("t", MaxTitleLen+1), "", 0, 0, "title"},
		"title in runes":    {strings.Repeat("日", MaxTitleLen), "", 0, 0, ""},
		"content at limit":  {"", strings.Repeat("c", MaxContentLen), 0, 0, ""},
		"content over":      {"", strings.Repeat("c", MaxContentLen+1), 0, 0, "content"},
		"too many accounts": {"", "", MaxTargets + 1, 0, "social_account_ids"},
		"max accounts":      {"", "", MaxTargets, 0, ""},
		"too many media":    {"", "", 0, MaxMedia + 1, "media_ids"},
	} {
		err := validateBasics(tc.title, tc.content, tc.accounts, tc.mids)
		if tc.field == "" && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if tc.field != "" {
			if e, ok := errs.As(err); !ok || e.Code != errs.Validation || e.Fields[tc.field] == "" {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestValidateScheduleTime(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	s := &Service{clock: fixedClock{now}}
	for name, tc := range map[string]struct {
		at time.Time
		ok bool
	}{
		"a minute ahead":      {now.Add(time.Minute), true},
		"now":                 {now, false},
		"past":                {now.Add(-time.Second), false},
		"just inside a year":  {now.Add(MaxScheduleIn - time.Minute), true},
		"exactly the limit":   {now.Add(MaxScheduleIn), true},
		"beyond a year":       {now.Add(MaxScheduleIn + time.Second), false},
		"other zone, in time": {now.Add(time.Hour).In(time.FixedZone("x", 5*3600)), true},
	} {
		err := s.validateScheduleTime(tc.at)
		if tc.ok && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if !tc.ok {
			if e, ok := errs.As(err); !ok || e.Code != errs.Validation || e.Fields["scheduled_at"] == "" {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}

func TestMediaIDsKeepsOrder(t *testing.T) {
	a, b := img(), vid()
	if got := mediaIDs([]media.Media{a, b}); len(got) != 2 || got[0] != a.ID || got[1] != b.ID {
		t.Fatalf("%v", got)
	}
}

func TestErrInvalidStatus(t *testing.T) {
	e, ok := errs.As(errInvalidStatus(post.Status("bogus")))
	if !ok || e.Code != errs.Validation || e.Fields["status"] == "" || !strings.Contains(e.Message, "bogus") {
		t.Fatalf("%v", e)
	}
}
