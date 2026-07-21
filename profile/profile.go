// Package profile defines immutable Markdown dialect configurations.
package profile

import (
	"fmt"
	"strings"
)

// Feature is a parser or renderer capability enabled by a profile.
type Feature uint64

const (
	Tables Feature = 1 << iota
	TaskLists
	Strikethrough
	ExtendedAutolinks
	TagFilter
	KoreanRangeInference
	PairedPunctuationEmphasis
	ParagraphEndRecovery
)

// Profile is an immutable Markdown dialect definition.
type Profile struct {
	id             string
	displayName    string
	commonMarkBase string
	features       Feature
}

var (
	// CommonMark0312 implements CommonMark 0.31.2 without extensions.
	CommonMark0312 = mustNew("commonmark-0.31.2", "CommonMark 0.31.2", "0.31.2", 0)
	// GFM029 implements the official GFM 0.29-gfm specification.
	GFM029 = mustNew("gfm-0.29", "GFM 0.29-gfm", "0.29", Tables|TaskLists|Strikethrough|ExtendedAutolinks|TagFilter)
	// GFM combines CommonMark 0.31.2 with the official GFM extensions.
	GFM = mustNew("gfm-modern", "GFM (CommonMark 0.31.2)", "0.31.2", Tables|TaskLists|Strikethrough|ExtendedAutolinks|TagFilter)
	// EnhanceMarkV1 adds deterministic intention-aware rules to modern GFM.
	EnhanceMarkV1 = mustNew("enhancemark-v1", "EnhanceMark v1", "0.31.2", Tables|TaskLists|Strikethrough|ExtendedAutolinks|TagFilter|KoreanRangeInference|PairedPunctuationEmphasis|ParagraphEndRecovery)
)

// New constructs a custom immutable profile for extension authors.
func New(id, displayName, commonMarkBase string, features Feature) (Profile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Profile{}, fmt.Errorf("profile: ID is required")
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.' {
			continue
		}
		return Profile{}, fmt.Errorf("profile: ID %q contains unsupported character %q", id, r)
	}
	if displayName == "" {
		displayName = id
	}
	if commonMarkBase == "" {
		return Profile{}, fmt.Errorf("profile: CommonMark base is required")
	}
	return Profile{id: id, displayName: displayName, commonMarkBase: commonMarkBase, features: features}, nil
}

func mustNew(id, displayName, commonMarkBase string, features Feature) Profile {
	result, err := New(id, displayName, commonMarkBase, features)
	if err != nil {
		panic(err)
	}
	return result
}

func (p Profile) ID() string             { return p.id }
func (p Profile) String() string         { return p.displayName }
func (p Profile) CommonMarkBase() string { return p.commonMarkBase }
func (p Profile) Features() Feature      { return p.features }
func (p Profile) Has(feature Feature) bool {
	return p.features&feature == feature
}
func (p Profile) Valid() bool { return p.id != "" }

// Parse resolves a CLI or configuration profile name.
func Parse(name string) (Profile, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "commonmark", "commonmark-0.31.2", "commonmark0312":
		return CommonMark0312, nil
	case "gfm029", "gfm-0.29", "gfm-0.29-gfm":
		return GFM029, nil
	case "gfm", "gfm-modern":
		return GFM, nil
	case "enhance", "enhancemark", "enhancemark-v1":
		return EnhanceMarkV1, nil
	default:
		return Profile{}, fmt.Errorf("profile: unknown profile %q", name)
	}
}
