package provider

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWithAccountLimitsKeepsTheStricterValue(t *testing.T) {
	base := Capabilities{MaxTextLength: 500, MaxMediaCount: 4, MaxImageBytes: 8 << 20}
	for name, tc := range map[string]struct {
		meta map[string]any
		want Capabilities
	}{
		"no limits":                    {nil, base},
		"empty metadata":               {map[string]any{"host": "x"}, base},
		"limits of the wrong type":     {map[string]any{"limits": "many"}, base},
		"lower text limit":             {map[string]any{"limits": map[string]any{"max_characters": 300}}, Capabilities{MaxTextLength: 300, MaxMediaCount: 4, MaxImageBytes: 8 << 20}},
		"higher text limit is ignored": {map[string]any{"limits": map[string]any{"max_characters": 5000}}, base},
		"lower media and image":        {map[string]any{"limits": map[string]any{"max_media": 2, "max_image_bytes": 1000}}, Capabilities{MaxTextLength: 500, MaxMediaCount: 2, MaxImageBytes: 1000}},
		"zero and negative ignored":    {map[string]any{"limits": map[string]any{"max_characters": 0, "max_media": -1, "max_image_bytes": 0}}, base},
		"fraction ignored":             {map[string]any{"limits": map[string]any{"max_characters": 10.5}}, base},
		"string ignored":               {map[string]any{"limits": map[string]any{"max_characters": "10"}}, base},
	} {
		if got := base.WithAccountLimits(tc.meta); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestWithAccountLimitsReadsJSONNumbers(t *testing.T) {
	var meta map[string]any
	if err := json.Unmarshal([]byte(`{"limits":{"max_characters":280,"max_media":1}}`), &meta); err != nil {
		t.Fatal(err)
	}
	got := Capabilities{MaxTextLength: 500, MaxMediaCount: 4}.WithAccountLimits(meta)
	if got.MaxTextLength != 280 || got.MaxMediaCount != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestWithAccountLimitsSetsALimitThePlatformLeftOpen(t *testing.T) {
	got := Capabilities{CanPublishText: true}.WithAccountLimits(map[string]any{"limits": map[string]any{"max_characters": 100, "max_image_bytes": 50}})
	if got.MaxTextLength != 100 || got.MaxImageBytes != 50 {
		t.Fatalf("%+v", got)
	}
	if got := (Capabilities{}).WithAccountLimits(map[string]any{"limits": map[string]any{"max_media": 3}}); got.MaxMediaCount != 0 {
		t.Fatal("an account limit must never grant media to a text-only network")
	}
}
