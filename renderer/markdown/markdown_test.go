package markdown_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	markmarkdown "github.com/gaon12/markonward/renderer/markdown"
)

func normalize(t *testing.T, selected profile.Profile, source string) string {
	t.Helper()
	p, err := parser.New(selected)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Parse(context.Background(), []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := markmarkdown.New(selected).Render(context.Background(), &output, result.Document); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestNormalizedMarkdownIsIdempotent(t *testing.T) {
	t.Parallel()
	source := "# title ###\n\n- [x] **done**\n\na|b\n---|:---:\n1|2\n\n````go\n```\n````\n"
	first := normalize(t, profile.GFM, source)
	second := normalize(t, profile.GFM, first)
	if second != first {
		t.Fatalf("normalization is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestAllSpaceCodeSpanIsIdempotent(t *testing.T) {
	t.Parallel()
	source := "` `00"
	first := normalize(t, profile.EnhanceMarkV1, source)
	second := normalize(t, profile.EnhanceMarkV1, first)
	if second != first {
		t.Fatalf("all-space code span normalization is not idempotent: first=%q second=%q", first, second)
	}
}

func TestEscapedClosingParenthesisDoesNotBecomeAList(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, `0\)`)
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0\\)\n" || second != first {
		t.Fatalf("escaped list marker normalization: first=%q second=%q", first, second)
	}
}

func TestDocumentLeadingIndentIsCanonicalized(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, " 0\x00!0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0\x00\\!0\n" || second != first {
		t.Fatalf("document leading indentation: first=%q second=%q", first, second)
	}
}

func TestOrderedListZeroStartIsPreserved(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0) item")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0. item\n" || second != first {
		t.Fatalf("zero-start list normalization: first=%q second=%q", first, second)
	}
}

func TestBlankCodeFenceLineInsideListHasNoGrowingIndent(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "* ```")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "- ```\n\n  ```\n" || second != first {
		t.Fatalf("blank fenced-code line in list: first=%q second=%q", first, second)
	}
}

func TestRecoveredMixedDelimiterNestingIsIdempotent(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "_0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "_00_\n" || second != first {
		t.Fatalf("mixed recovered delimiter normalization: first=%q second=%q", first, second)
	}
}

func TestLiteralURLPrefixDoesNotBecomeAnAutolink(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, " http://!")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "http\\://\\!\n" || second != first {
		t.Fatalf("literal URL prefix normalization: first=%q second=%q", first, second)
	}
}

func TestInvalidAngleAutolinkDoesNotAbsorbFollowingEscape(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "http://>!")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "http://>&#33;\n" || second != first {
		t.Fatalf("invalid-angle autolink boundary: first=%q second=%q", first, second)
	}
}

func TestAdjacentEmphasisDelimitersRemainDistinct(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0*0**!")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0*0\\*\\*\\!*\n" || second != first {
		t.Fatalf("adjacent emphasis normalization: first=%q second=%q", first, second)
	}
}

func TestMergedFormattingTreatsMemberWhitespaceAsInterior(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0** \x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*0 \x00*&#48;\n" || second != first {
		t.Fatalf("merged member whitespace: first=%q second=%q", first, second)
	}
}

func TestRecoveredSingleTildeNestingIsIdempotent(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "~!~00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "~\\!00~\n" || second != first {
		t.Fatalf("single-tilde recovery normalization: first=%q second=%q", first, second)
	}
}

func TestRecoveredDuplicateEmphasisCollapsesBeforeRuleOfThree(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*!*0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*\\!0*&#48;\n" || second != first {
		t.Fatalf("rule-of-three recovery normalization: first=%q second=%q", first, second)
	}
}

func TestDeepRecoveredFormattingAlternatesDelimiters(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, " *!*0**00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "\\!000\n" || second != first {
		t.Fatalf("deep recovered formatting normalization: first=%q second=%q", first, second)
	}
}

func TestNestedIntrawordEmphasisUsesStableBoundaryEntity(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "**0*0***\n" || second != first {
		t.Fatalf("nested intraword emphasis normalization: first=%q second=%q", first, second)
	}
}

func TestNestedFormattingCombinesRunsAtWordBoundaries(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0***0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "&#48;***0***\n" || second != first {
		t.Fatalf("combined formatting-run normalization: first=%q second=%q", first, second)
	}
}

func TestDeepStrongFormattingCombinesRunsAtWordBoundaries(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**0****0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "00\n" || second != first {
		t.Fatalf("deep combined strong-run normalization: first=%q second=%q", first, second)
	}
}

func TestFormattingEdgeWhitespaceUsesStableEntities(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0\f")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*0&#12;*\n" || second != first {
		t.Fatalf("formatting-edge whitespace normalization: first=%q second=%q", first, second)
	}
}

func TestFormattingEdgeWhitespaceIgnoresTextNodeSplits(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0 \f")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*0&#32;&#12;*\n" || second != first {
		t.Fatalf("split formatting-edge whitespace normalization: first=%q second=%q", first, second)
	}
}

func TestLiteralCarriageReturnDoesNotBecomeALineEnding(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0\r ")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0&#13;\n" || second != first {
		t.Fatalf("literal carriage-return normalization: first=%q second=%q", first, second)
	}
}

func TestNULBeforeFormattingIsPreserved(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0\x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0*\x000*\n" || second != first {
		t.Fatalf("NUL formatting-boundary normalization: first=%q second=%q", first, second)
	}
}

func TestControlAfterFormattingUsesStableEntity(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**!*000*\x04")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "**\\!_000_&#4;**\n" || second != first {
		t.Fatalf("control formatting-boundary normalization: first=%q second=%q", first, second)
	}
}

func TestNULBeforeNestedFormattingKeepsLiteralContent(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**\x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***\x000***\n" || second != first {
		t.Fatalf("NUL nested-formatting normalization: first=%q second=%q", first, second)
	}
}

func TestLeadingControlMoveProtectsOuterFormattingOpener(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0**\x00*0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "&#48;**_\x000_&#48;**\n" || second != first {
		t.Fatalf("outer opener before moved leading control: first=%q second=%q", first, second)
	}
}

func TestAdjustedParentMarkerProtectsOuterFormattingOpener(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "\x000***0**0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "\x00&#48;*__0__&#48;*\n" || second != first {
		t.Fatalf("adjusted parent marker opener: first=%q second=%q", first, second)
	}
}

func TestMovedPrefixControlStillProtectsOuterFormattingOpener(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0\x00***0*00**")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "&#48;**_\x000_&#48;0**\n" || second != first {
		t.Fatalf("outer opener after moved prefix control: first=%q second=%q", first, second)
	}
}

func TestAdjacentNestedRecoveryWithMovedControlUsesSafeFlattening(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0***\x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0\x000\n" || second != first {
		t.Fatalf("nested emphasis after moved control: first=%q second=%q", first, second)
	}
}

func TestAdjacentNestedRecoveryUsesSafeFlattening(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*\x00****0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "\x000\n" || second != first {
		t.Fatalf("recovered nested-run normalization: first=%q second=%q", first, second)
	}
}

func TestCollapsedDuplicateFormattingKeepsUnrepresentableControl(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0*0 *0*\x00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0*0 0*\x00\n" || second != first {
		t.Fatalf("unrepresentable control boundary normalization: first=%q second=%q", first, second)
	}
}

func TestDeepRecoveredStrongCombinesRuns(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "******!")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "\\!\n" || second != first {
		t.Fatalf("empty recovered formatting normalization: first=%q second=%q", first, second)
	}
}

func TestNULMovesInsideCombinedFormattingRun(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "\x00***0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***\x000***\n" || second != first {
		t.Fatalf("NUL combined-formatting normalization: first=%q second=%q", first, second)
	}
}

func TestExtendedEmailThatIsNotAnAngleAutolinkStaysBare(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0@.0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0@.0\n" || second != first {
		t.Fatalf("extended email autolink normalization: first=%q second=%q", first, second)
	}
}

func TestTrimmedExtendedEmailUsesExplicitLink(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, ".@0.")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "[\\.@0](mailto:.@0)\\.\n" || second != first {
		t.Fatalf("trimmed extended email normalization: first=%q second=%q", first, second)
	}
}

func TestTrimmedExtendedEmailAfterEscapedBoundaryUsesExplicitLink(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "<0.@a.")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "\\<[0\\.@a](mailto:0.@a)\\.\n" || second != first {
		t.Fatalf("boundary-prefixed extended email: first=%q second=%q", first, second)
	}
}

func TestExtendedURLWithAngleCharacterStaysBare(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "http://>")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "http://>\n" || second != first {
		t.Fatalf("extended URL autolink normalization: first=%q second=%q", first, second)
	}
}

func TestAdjacentNestedEmphasisUsesDistinctMarkers(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*!***0**0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "\\!00\n" || second != first {
		t.Fatalf("adjacent nested emphasis normalization: first=%q second=%q", first, second)
	}
}

func TestNULBeforeFormattingKeepsFirstContentRuneLiteral(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "\x00*0**0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*\x000**0***\n" || second != first {
		t.Fatalf("NUL formatting-opener normalization: first=%q second=%q", first, second)
	}
}

func TestMixedDirectNestingDoesNotReuseOuterMarker(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "_!____*!")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "\\!\\!\n" || second != first {
		t.Fatalf("mixed direct nesting normalization: first=%q second=%q", first, second)
	}
}

func TestUnrepresentableControlMovesInsideOpeningDelimiter(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "****\x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "\x000\n" || second != first {
		t.Fatalf("control opening-boundary normalization: first=%q second=%q", first, second)
	}
}

func TestMovedOpeningControlMakesFollowingSpaceStable(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "\x00* \x00*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*\x00 \x00*\n" || second != first {
		t.Fatalf("moved control and space normalization: first=%q second=%q", first, second)
	}
}

func TestRecoveredParentKeepsDistinctMarkerAcrossMovedControl(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**\x00*0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "**_\x000_&#48;**\n" || second != first {
		t.Fatalf("recovered control-boundary normalization: first=%q second=%q", first, second)
	}
}

func TestMovedControlKeepsDistinctMarkerWhenContentFollows(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**\x00*0*0**0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "**_\x000_&#48;**&#48;\n" || second != first {
		t.Fatalf("moved control with following content: first=%q second=%q", first, second)
	}
}

func TestRecoveredNestedStrongBeforeControlUsesDistinctSiblingMarker(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "****0***0*\x00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "**0**_0\x00_\n" || second != first {
		t.Fatalf("recovered nested strong before control: first=%q second=%q", first, second)
	}
}

func TestNestedEmphasisDoesNotCombineAcrossFollowingPeer(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**\x00*0****0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "**_\x000_**_0_\n" || second != first {
		t.Fatalf("nested emphasis before following peer: first=%q second=%q", first, second)
	}
}

func TestMovedLeadingControlKeepsDistinctMarkerAcrossMergedMembers(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "\x00***0**0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*__\x000__&#48;*\n" || second != first {
		t.Fatalf("leading control in merged formatting: first=%q second=%q", first, second)
	}
}

func TestMovedControlLetsOnlyNestedChildCombineDelimiterRun(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**\x00*0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***\x000***\n" || second != first {
		t.Fatalf("moved-control only-child normalization: first=%q second=%q", first, second)
	}
}

func TestDeepNestedFormattingKeepsWordLikeBoundaryLiteral(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0 *!*0**0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0 \\!00\n" || second != first {
		t.Fatalf("deep word-like formatting boundary: first=%q second=%q", first, second)
	}
}

func TestAsteriskFallbackHonorsRuleOfThree(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "****0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "00\n" || second != first {
		t.Fatalf("asterisk rule-of-three normalization: first=%q second=%q", first, second)
	}
}

func TestAdjacentListsKeepDistinctMarkers(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*\n+ 0000")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "-\n\n+ 0000\n" || second != first {
		t.Fatalf("adjacent list normalization: first=%q second=%q", first, second)
	}
}

func TestNestedEmptyListsDoNotBecomeThematicBreak(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "+ * *")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "- + *\n" || second != first {
		t.Fatalf("nested empty-list normalization: first=%q second=%q", first, second)
	}
}

func TestOuterBoundaryAccountsForEntityOnlyFirstChild(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0*0*****0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "000\n" || second != first {
		t.Fatalf("entity-only first-child normalization: first=%q second=%q", first, second)
	}
}

func TestDuplicateStrongLayersCollapseAcrossTextSiblings(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "00**0****0**")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "00**00**\n" || second != first {
		t.Fatalf("inline entity-only first-child boundary: first=%q second=%q", first, second)
	}
}

func TestRecoveredDuplicateEmphasisCollapsesAfterWord(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0*_\x00*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0*\x00*\n" || second != first {
		t.Fatalf("nested opening delimiter boundary: first=%q second=%q", first, second)
	}
}

func TestCombinedDelimiterRunCanOpenAfterWord(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0**\x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0***\x000***\n" || second != first {
		t.Fatalf("combined opening delimiter run: first=%q second=%q", first, second)
	}
}

func TestNestedStrikethroughAcrossEmphasisIsCollapsed(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "~*~0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0\n" || second != first {
		t.Fatalf("nested strikethrough normalization: first=%q second=%q", first, second)
	}
}

func TestBranchedRecoveredStrikethroughUsesSafeFlattening(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "~0*0*~0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "000\n" || second != first {
		t.Fatalf("collapsed strikethrough boundary: first=%q second=%q", first, second)
	}
}

func TestCollapsedNestedStrikethroughDoesNotAddTrailingEntity(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "~~0 ~0~0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "~~0 00~~\n" || second != first {
		t.Fatalf("collapsed strikethrough trailing boundary: first=%q second=%q", first, second)
	}
}

func TestFormattingSeparatedByMovedControlUsesDistinctMarkers(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**0**\x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "**0**_\x000_\n" || second != first {
		t.Fatalf("control-separated formatting normalization: first=%q second=%q", first, second)
	}
}

func TestSameFormattingSeparatedByControlIsMerged(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0*\x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*0\x000*\n" || second != first {
		t.Fatalf("control-separated matching formatting: first=%q second=%q", first, second)
	}
}

func TestAdjacentRecoveredFormattingUsesSafeFlattening(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0**!*0**!")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0\\!0\\!\n" || second != first {
		t.Fatalf("adjacent formatting-group normalization: first=%q second=%q", first, second)
	}
}

func TestMergedEmphasisGroupCombinesWithStrongParent(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**_0_*0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***00***\n" || second != first {
		t.Fatalf("merged emphasis group under strong: first=%q second=%q", first, second)
	}
}

func TestMergedStrongGroupCombinesAcrossTrailingControl(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*__0__**0**\x00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***00**\x00*\n" || second != first {
		t.Fatalf("merged strong group before trailing control: first=%q second=%q", first, second)
	}
}

func TestRecoveredStrongCombinesAcrossCollapsedControlSibling(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*__\x00_**0\x00_")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***\x000\x00***\n" || second != first {
		t.Fatalf("recovered strong after collapsed control sibling: first=%q second=%q", first, second)
	}
}

func TestMergedGroupControlBoundaryDoesNotAffectNestedDelimiter(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*!*00*0*\x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*\\!_00_&#48;\x000*\n" || second != first {
		t.Fatalf("merged group control after nested formatting: first=%q second=%q", first, second)
	}
}

func TestMergedFormattingGroupSharesSimpleNestedLayer(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0******0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***00***\n" || second != first {
		t.Fatalf("merged group with shared nested formatting: first=%q second=%q", first, second)
	}
}

func TestMergedStrongGroupCombinesWithParentAroundNestedEmphasis(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "****0*0****0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***_0_&#48;0***&#48;\n" || second != first {
		t.Fatalf("merged strong group with nested emphasis: first=%q second=%q", first, second)
	}
}

func TestControlOnlyMergedMemberMovesInsideNestedDelimiter(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*!*0**_\x00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*\\!_0\x00_*\n" || second != first {
		t.Fatalf("control-only merged member after nested emphasis: first=%q second=%q", first, second)
	}
}

func TestRecoveredStrongFactorsSharedEmphasisSibling(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0***_0_")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*0**0***\n" || second != first {
		t.Fatalf("recovered strong with shared emphasis sibling: first=%q second=%q", first, second)
	}
}

func TestMergedFormattingGroupSharesNestedLayerAfterTextPrefix(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0**0****0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*0**00***\n" || second != first {
		t.Fatalf("merged group with prefixed shared formatting: first=%q second=%q", first, second)
	}
}

func TestMergedFormattingGroupSharesNestedLayerBeforeTextSuffix(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0******0**0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*__00__&#48;*\n" || second != first {
		t.Fatalf("merged group with suffixed shared formatting: first=%q second=%q", first, second)
	}
}

func TestNestedStrikethroughDelimiterKeepsLeadingEntity(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0*\x00~0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "&#48;*~\x000~*\n" || second != first {
		t.Fatalf("entity before emphasis containing strikethrough: first=%q second=%q", first, second)
	}
}

func TestMergedGroupKeepsNestedMarkerAfterEarlierMember(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "___0_*_0_*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "_**0**_0__\n" || second != first {
		t.Fatalf("nested emphasis after earlier merged member: first=%q second=%q", first, second)
	}
}

func TestMergedGroupAlternatesNestedMarkerAfterPlainContent(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0**!*0**!*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*0\\!_0\\!_*\n" || second != first {
		t.Fatalf("nested emphasis after plain merged content: first=%q second=%q", first, second)
	}
}

func TestCollapsedFormattingUsesRenderedWhitespaceEdge(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*__&}}}}\x00_ 00\x00_")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***&\\}\\}\\}\\}\x00 00\x00***\n" || second != first {
		t.Fatalf("collapsed formatting whitespace edge: first=%q second=%q", first, second)
	}
}

func TestMergedFormattingGroupSharesSplitTextNestedLayer(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0******!!0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***0\\!\\!0***\n" || second != first {
		t.Fatalf("merged group with split nested text: first=%q second=%q", first, second)
	}
}

func TestDeepNestedEmphasisProtectsDistantAncestorMarker(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**0*0*0*!0*000")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*_&#48;_0_&#48;_\\!0*&#48;00\n" || second != first {
		t.Fatalf("deep nested emphasis marker boundary: first=%q second=%q", first, second)
	}
}

func TestSharedNestedLayerAfterPrefixUsesAlternateMarker(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*!**0****0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*\\!__00__*\n" || second != first {
		t.Fatalf("shared nested layer after prefix: first=%q second=%q", first, second)
	}
}

func TestRecoveredFactoringWithPunctuationIsFlattened(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "_0_*_!0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0\\!0\n" || second != first {
		t.Fatalf("punctuation recovered factoring: first=%q second=%q", first, second)
	}
}

func TestControlContentStillHonorsRuleOfThree(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "****0*\x00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0\x00\n" || second != first {
		t.Fatalf("control-content rule-of-three normalization: first=%q second=%q", first, second)
	}
}

func TestStrongRunDoesNotAbsorbNestedEmphasisChain(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**_0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "00\n" || second != first {
		t.Fatalf("strong and nested emphasis normalization: first=%q second=%q", first, second)
	}
}

func TestCombinedOuterRunAlternatesNestedEmphasisMarker(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "****!0*000***")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***_\\!0_&#48;00***\n" || second != first {
		t.Fatalf("combined outer run with nested emphasis: first=%q second=%q", first, second)
	}
}

func TestMergedFormattingIgnoresEscapedDelimiterBoundary(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "_*_*00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "_\\*00_\n" || second != first {
		t.Fatalf("escaped merged-boundary normalization: first=%q second=%q", first, second)
	}
}

func TestNestedFormattingUsesActualMergedParentMarker(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0 *0**0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0 00\n" || second != first {
		t.Fatalf("actual merged-parent marker normalization: first=%q second=%q", first, second)
	}
}

func TestMergedGroupCanonicalizesDirectStrongRun(t *testing.T) {
	t.Parallel()
	source := "__*_ * *!_*!       ***00000"
	first := normalize(t, profile.EnhanceMarkV1, source)
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != second {
		t.Fatalf("merged direct-strong normalization: first=%q second=%q", first, second)
	}
}

func TestMergedStrongGroupCombinesWithItsEmphasisParent(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0****0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***00***&#48;\n" || second != first {
		t.Fatalf("merged strong-parent combination: first=%q second=%q", first, second)
	}
}

func TestMergedFormattingMovesControlOnlyRemainderBeforeNestedCloser(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0**\x00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***0\x00***\n" || second != first {
		t.Fatalf("control-only group remainder: first=%q second=%q", first, second)
	}
}

func TestNestedCloserBeforeControlUsesAsteriskFallback(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0**\x000")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***0**\x000*\n" || second != first {
		t.Fatalf("control-following closer normalization: first=%q second=%q", first, second)
	}
}

func TestBranchedRecoveryAcrossControlUsesSafeFlattening(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "****0**\x00*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0\x000\n" || second != first {
		t.Fatalf("control-separated recovered duplicate: first=%q second=%q", first, second)
	}
}

func TestBranchedNestedRecoveryUsesSafeFlattening(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*!_0_**!")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "\\!0\\!\n" || second != first {
		t.Fatalf("branched nested recovery: first=%q second=%q", first, second)
	}
}

func TestImmediateEmphasisParentWinsMarkerSelection(t *testing.T) {
	t.Parallel()
	source := "*****!*0 *!"
	first := normalize(t, profile.EnhanceMarkV1, source)
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != second {
		t.Fatalf("immediate emphasis-parent normalization: first=%q second=%q", first, second)
	}
}

func TestAdjacentNestedFormattingUsesDistinctOpeningMarker(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*!*0***0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*\\!_0_**0***\n" || second != first {
		t.Fatalf("adjacent nested formatting normalization: first=%q second=%q", first, second)
	}
}

func TestDuplicateRecoveredFormattingLayersAreCollapsed(t *testing.T) {
	t.Parallel()
	source := "*******! *0"
	first := normalize(t, profile.EnhanceMarkV1, source)
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != second {
		t.Fatalf("duplicate recovered-layer normalization: first=%q second=%q", first, second)
	}
}

func TestValidFormattingInsideDuplicateRecoveredLayerIsCollapsed(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "* *0*_*0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "- *00*\n" || second != first {
		t.Fatalf("valid formatting under recovered duplicate: first=%q second=%q", first, second)
	}
}

func TestCollapsedRecoveredSiblingDoesNotAddEntityBoundary(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "_*0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "_00_\n" || second != first {
		t.Fatalf("collapsed sibling entity boundary: first=%q second=%q", first, second)
	}
}

func TestMergedMemberCollapsedFormattingKeepsDelimiterBoundary(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0*_*0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*__0__&#48;*\n" || second != first {
		t.Fatalf("merged collapsed-member boundary: first=%q second=%q", first, second)
	}
}

func TestCollapsedRecoveredSiblingRetainsNestedCloserBoundary(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "_0*0**0*0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "_00**0**&#48;_\n" || second != first {
		t.Fatalf("nested closer in collapsed sibling: first=%q second=%q", first, second)
	}
}

func TestCollapsedParentUsesRenderedSiblingContextForNestedStrong(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0*****0*****0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "*&#48;____0____&#48;*\n" || second != first {
		t.Fatalf("rendered sibling context for nested strong: first=%q second=%q", first, second)
	}
}

func TestBranchedRecoveredStrongUsesSafeFlattening(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "****0**0**00")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0000\n" || second != first {
		t.Fatalf("text before collapsed sibling: first=%q second=%q", first, second)
	}
}

func TestCollapsedEmphasisDescendantDoesNotBlockCombinedRun(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "**_*0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "***0***\n" || second != first {
		t.Fatalf("collapsed emphasis descendant: first=%q second=%q", first, second)
	}
}

func TestRuleOfThreeUsesRenderedAncestorStack(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "****0*0*")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "**0*0***\n" || second != first {
		t.Fatalf("rendered ancestor rule-of-three: first=%q second=%q", first, second)
	}
}

func TestBoundaryPredictionSkipsCollapsedRecovery(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0****0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "0**0**\n" || second != first {
		t.Fatalf("collapsed recovery boundary prediction: first=%q second=%q", first, second)
	}
}

func TestMergedGroupStrongAccountsForEarlierMembers(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0****!")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != second {
		t.Fatalf("merged earlier-member normalization: first=%q second=%q", first, second)
	}
}

func TestRecoveredLayerThatCouldFactorAcrossSiblingsIsFlattened(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "*0****0**")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "00\n" || second != first {
		t.Fatalf("ambiguous recovered factoring: first=%q second=%q", first, second)
	}
}

func TestParallelRecoveredLayersThatCouldFactorAreFlattened(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0****0**")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "00\\*\n" || second != first {
		t.Fatalf("parallel recovered factoring: first=%q second=%q", first, second)
	}
}

func TestMergedGroupWithNestedRecoveryUsesSafeFlattening(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "0*0****!")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "00\\!\n" || second != first {
		t.Fatalf("merged opening-boundary normalization: first=%q second=%q", first, second)
	}
}

func TestThreeRecoveredSiblingsTriggerSafeFlattening(t *testing.T) {
	t.Parallel()
	first := normalize(t, profile.EnhanceMarkV1, "***0****0")
	second := normalize(t, profile.EnhanceMarkV1, first)
	if first != "00\n" || second != first {
		t.Fatalf("recovered sibling flattening: first=%q second=%q", first, second)
	}
}

func TestRecoveredEnhanceFormattingGetsExplicitCloser(t *testing.T) {
	t.Parallel()
	got := normalize(t, profile.EnhanceMarkV1, "**unfinished")
	if got != "**unfinished**\n" {
		t.Fatalf("normalized recovery = %q", got)
	}
}
