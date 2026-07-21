package profile_test

import (
	"testing"

	"github.com/gaon12/markonward/profile"
)

func TestBuiltInProfiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		profile profile.Profile
		feature profile.Feature
		want    bool
	}{
		{name: "commonmark_has_no_tables", profile: profile.CommonMark0312, feature: profile.Tables},
		{name: "gfm_has_tables", profile: profile.GFM, feature: profile.Tables, want: true},
		{name: "official_gfm_uses_029", profile: profile.GFM029, feature: profile.TagFilter, want: true},
		{name: "enhance_has_korean_ranges", profile: profile.EnhanceMarkV1, feature: profile.KoreanRangeInference, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.profile.Has(test.feature); got != test.want {
				t.Fatalf("Has(%v) = %v, want %v", test.feature, got, test.want)
			}
		})
	}
}

func TestParseRequiresKnownProfile(t *testing.T) {
	t.Parallel()
	if got, err := profile.Parse("enhance"); err != nil || got.ID() != profile.EnhanceMarkV1.ID() {
		t.Fatalf("Parse(enhance) = %#v, %v", got, err)
	}
	if _, err := profile.Parse("mystery"); err == nil {
		t.Fatal("Parse(mystery) should fail")
	}
}
