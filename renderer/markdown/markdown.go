// Package markdown renders AST documents as normalized Markdown.
package markdown

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/mail"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/extension"
	"github.com/gaon12/markonward/profile"
	baserenderer "github.com/gaon12/markonward/renderer"
)

// Renderer emits deterministic Markdown for one target profile.
type Renderer struct {
	profile  profile.Profile
	handlers baserenderer.ExtensionSet
}

// New constructs a renderer. An invalid profile is reported by Render.
func New(selected profile.Profile) *Renderer { return &Renderer{profile: selected} }

// NewWithExtensions constructs a profile-aware renderer with immutable
// custom-node handlers.
func NewWithExtensions(selected profile.Profile, extensions ...extension.Extension) (*Renderer, error) {
	handlers, err := baserenderer.CompileExtensions(extensions...)
	if err != nil {
		return nil, err
	}
	return &Renderer{profile: selected, handlers: handlers}, nil
}

// Render writes normalized Markdown.
func (r *Renderer) Render(ctx context.Context, writer io.Writer, document *ast.Document) error {
	if r == nil || !r.profile.Valid() {
		return fmt.Errorf("markdown: a target profile is required")
	}
	if ctx == nil || writer == nil || document == nil {
		return fmt.Errorf("markdown: context, writer, and document are required")
	}
	state := newRenderState(r, ctx, document)
	if err := state.blocks(document.Root()); err != nil {
		return err
	}
	result := strings.TrimRight(state.output.String(), " \t\n")
	if result != "" {
		result += "\n"
	}
	_, err := io.WriteString(writer, result)
	return err
}

type renderState struct {
	renderer *Renderer
	ctx      context.Context
	document *ast.Document
	output   strings.Builder
	skipText ast.NodeID
	skipByte int

	inlineMarkers  []byte
	inlineContent  []uint8
	prefixedInline []bool
	inlineStack    []inlineFrame
}

type inlineFrame struct {
	owner            ast.NodeID
	kind             ast.Kind
	marker           byte
	merged           bool
	hasPreceding     bool
	hasFollowing     bool
	followingControl bool
}

func newRenderState(renderer *Renderer, ctx context.Context, document *ast.Document) renderState {
	return renderState{
		renderer:       renderer,
		ctx:            ctx,
		document:       document,
		inlineMarkers:  make([]byte, document.Len()+1),
		inlineContent:  make([]uint8, document.Len()+1),
		prefixedInline: make([]bool, document.Len()+1),
	}
}

func (s *renderState) blocks(parent ast.NodeID) error {
	for child := s.document.Node(parent).FirstChild(); child != ast.NoNode; child = s.document.Node(child).NextSibling() {
		if err := s.block(child); err != nil {
			return err
		}
	}
	return nil
}

func (s *renderState) block(id ast.NodeID) error { //nolint:gocyclo // The switch is the normalized block grammar table.
	if err := s.ctx.Err(); err != nil {
		return err
	}
	node := s.document.Node(id)
	switch node.Kind() {
	case ast.Paragraph:
		// Deep or factorable recovery trees can have several source spellings
		// whose delimiter stacks parse differently. Preserve their visible
		// content instead of emitting a normalization that changes on pass two.
		recoveredCount := s.countRecoveredFormatting(id)
		if s.hasAmbiguousRecoveredPath(id, 0) || recoveredCount >= 3 || (recoveredCount >= 2 && s.hasAmbiguousNestedRecovery(id)) || s.hasAmbiguousRecoveredFactoring(id) {
			if err := s.flattenFormatting(id); err != nil {
				return err
			}
			s.blankLine()
			break
		}
		if err := s.inlines(id); err != nil {
			return err
		}
		s.blankLine()
	case ast.Heading:
		level, _ := node.Integers()
		if level < 1 || level > 6 {
			return fmt.Errorf("markdown: invalid heading level %d", level)
		}
		s.output.WriteString(strings.Repeat("#", level) + " ")
		if err := s.inlines(id); err != nil {
			return err
		}
		s.blankLine()
	case ast.BlockQuote:
		content, err := s.renderBlocksToString(id)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
			s.output.WriteString(">")
			if line != "" {
				s.output.WriteByte(' ')
				s.output.WriteString(line)
			}
			s.output.WriteByte('\n')
		}
		s.blankLine()
	case ast.List:
		if err := s.renderList(id, 0); err != nil {
			return err
		}
		s.blankLine()
	case ast.ListItem:
		return fmt.Errorf("markdown: list item outside list")
	case ast.ThematicBreak:
		s.output.WriteString("---\n\n")
	case ast.CodeBlock:
		fence := strings.Repeat("`", maxInt(3, longestRun(node.Text(), '`')+1))
		s.output.WriteString(fence)
		if info := strings.TrimSpace(node.Title()); info != "" {
			s.output.WriteString(info)
		}
		s.output.WriteByte('\n')
		s.output.WriteString(node.Text())
		if !strings.HasSuffix(node.Text(), "\n") {
			s.output.WriteByte('\n')
		}
		s.output.WriteString(fence + "\n\n")
	case ast.HTMLBlock:
		s.output.WriteString(node.Text())
		s.blankLine()
	case ast.Table:
		if !s.renderer.profile.Has(profile.Tables) {
			return fmt.Errorf("markdown: target profile %s does not support tables", s.renderer.profile.ID())
		}
		if err := s.renderTable(id); err != nil {
			return err
		}
		s.blankLine()
	case ast.Invalid, ast.DocumentKind, ast.Text, ast.SoftBreak, ast.HardBreak, ast.CodeSpan, ast.Emphasis, ast.Strong, ast.Strikethrough, ast.Link, ast.Image, ast.Autolink, ast.RawHTML, ast.TableHead, ast.TableBody, ast.TableRow, ast.TableCell, ast.TaskCheck:
		return fmt.Errorf("markdown: unexpected block node %s", node.Kind())
	case ast.Custom:
		return s.custom(id, func(parent ast.NodeID) error { return s.blocks(parent) })
	default:
		return fmt.Errorf("markdown: unsupported node %s", node.Kind())
	}
	return nil
}

func (s *renderState) countRecoveredFormatting(parent ast.NodeID) int {
	count := 0
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		if isFormattingKind(child.Kind()) && child.Flags()&ast.InlineRecoveredDelimiter != 0 {
			count++
		}
		count += s.countRecoveredFormatting(childID)
	}
	return count
}

func (s *renderState) hasAmbiguousRecoveredPath(parent ast.NodeID, depth int) bool {
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		childDepth := depth
		if isFormattingKind(child.Kind()) && child.Flags()&ast.InlineRecoveredDelimiter != 0 {
			childDepth++
			if childDepth >= 3 {
				return true
			}
		}
		if s.hasAmbiguousRecoveredPath(childID, childDepth) {
			return true
		}
	}
	return false
}

func (s *renderState) hasAmbiguousNestedRecovery(parent ast.NodeID) bool {
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		branched := s.hasMultipleFormattingChildren(childID)
		if isFormattingKind(child.Kind()) && child.Flags()&ast.InlineRecoveredDelimiter != 0 && s.hasRecoveredFormattingDescendant(childID) && (branched || s.hasFormattingPeer(child)) {
			return true
		}
		if s.hasAmbiguousNestedRecovery(childID) {
			return true
		}
	}
	return false
}

func (s *renderState) hasMultipleFormattingChildren(parent ast.NodeID) bool {
	found := false
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		if !isFormattingKind(s.document.Node(childID).Kind()) {
			continue
		}
		if found {
			return true
		}
		found = true
	}
	return false
}

func (s *renderState) hasRecoveredFormattingDescendant(parent ast.NodeID) bool {
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		if (isFormattingKind(child.Kind()) && child.Flags()&ast.InlineRecoveredDelimiter != 0) || s.hasRecoveredFormattingDescendant(childID) {
			return true
		}
	}
	return false
}

func (s *renderState) hasFormattingPeer(node ast.Node) bool {
	for siblingID := node.PreviousSibling(); siblingID != ast.NoNode; siblingID = s.document.Node(siblingID).PreviousSibling() {
		sibling := s.document.Node(siblingID)
		if isFormattingKind(sibling.Kind()) {
			return true
		}
		if sibling.Kind() != ast.Text || !onlyUnrepresentableControls(sibling.Text()) {
			break
		}
	}
	for siblingID := node.NextSibling(); siblingID != ast.NoNode; siblingID = s.document.Node(siblingID).NextSibling() {
		sibling := s.document.Node(siblingID)
		if isFormattingKind(sibling.Kind()) {
			return true
		}
		if sibling.Kind() != ast.Text || !onlyUnrepresentableControls(sibling.Text()) {
			break
		}
	}
	return false
}

func (s *renderState) hasAmbiguousRecoveredFactoring(parent ast.NodeID) bool {
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		if isFormattingKind(child.Kind()) && child.FirstChild() != ast.NoNode && child.FirstChild() == child.LastChild() {
			nested := s.document.Node(child.FirstChild())
			if isFormattingKind(nested.Kind()) && nested.Flags()&ast.InlineRecoveredDelimiter != 0 {
				previous := child.PreviousSibling()
				next := child.NextSibling()
				if (previous != ast.NoNode && s.document.Node(previous).Kind() == nested.Kind()) || (next != ast.NoNode && s.document.Node(next).Kind() == nested.Kind()) {
					return true
				}
				if next != ast.NoNode {
					nextNode := s.document.Node(next)
					nextNestedID := nextNode.FirstChild()
					if nextNode.Kind() == child.Kind() && nextNestedID != ast.NoNode && nextNestedID == nextNode.LastChild() && s.document.Node(nextNestedID).Kind() == nested.Kind() {
						return true
					}
				}
			}
		}
		if s.hasAmbiguousRecoveredFactoring(childID) {
			return true
		}
	}
	return false
}

func (s *renderState) flattenFormatting(parent ast.NodeID) error {
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		if isFormattingKind(child.Kind()) {
			if err := s.flattenFormatting(childID); err != nil {
				return err
			}
			continue
		}
		if child.Kind() == ast.Text {
			s.output.WriteString(escapeText(child.Text()))
			continue
		}
		if err := s.inline(childID); err != nil {
			return err
		}
	}
	return nil
}

func (s *renderState) inlines(parent ast.NodeID) error {
	for child := s.document.Node(parent).FirstChild(); child != ast.NoNode; {
		last := s.lastMergeableFormattingSibling(child)
		if len(s.inlineStack) != 0 && s.inlineStack[len(s.inlineStack)-1].owner == parent {
			s.inlineStack[len(s.inlineStack)-1].hasPreceding = child != s.document.Node(parent).FirstChild()
			s.inlineStack[len(s.inlineStack)-1].hasFollowing = last != s.document.Node(parent).LastChild()
		}
		if last != child {
			if err := s.inlineFormattingGroup(child, last); err != nil {
				return err
			}
			child = s.document.Node(last).NextSibling()
			continue
		}
		if err := s.inline(child); err != nil {
			return err
		}
		child = s.document.Node(child).NextSibling()
	}
	return nil
}

func (s *renderState) lastMergeableFormattingSibling(first ast.NodeID) ast.NodeID {
	kind := s.document.Node(first).Kind()
	if kind != ast.Emphasis && kind != ast.Strong {
		return first
	}
	last := first
	for next := s.document.Node(last).NextSibling(); next != ast.NoNode; next = s.document.Node(last).NextSibling() {
		nextNode := s.document.Node(next)
		if nextNode.Kind() == kind {
			last = next
			continue
		}
		if nextNode.Kind() != ast.Text || !onlyUnrepresentableControls(nextNode.Text()) {
			break
		}
		after := nextNode.NextSibling()
		if after == ast.NoNode || s.document.Node(after).Kind() != kind {
			break
		}
		last = after
	}
	return last
}

func (s *renderState) inlineFormattingGroup(first, last ast.NodeID) error {
	// Adjacent nodes with the same formatting semantics are one canonical
	// delimiter run. Factoring the shared layer avoids ambiguous closer/opener
	// runs while retaining stronger formatting within individual members.
	firstNode := s.document.Node(first)
	length := delimiterLength(firstNode.Kind())
	delimiter := s.inlineDelimiter(firstNode, length)
	if firstNode.Kind() == ast.Strong && len(s.inlineStack) != 0 {
		parentFrame := s.inlineStack[len(s.inlineStack)-1]
		parent := s.document.Node(firstNode.Parent())
		if parentFrame.kind == ast.Emphasis && parent.FirstChild() == first && parent.LastChild() == last && !s.formattingGroupHasEmphasisDescendant(first, last) {
			delimiter = strings.Repeat(string(parentFrame.marker), length)
		}
	}
	s.stabilizeFormattingGroupOpeningBoundary(first, last, delimiter[0])
	boundary := s.takeTrailingUnrepresentableControls()
	s.output.WriteString(delimiter)
	s.output.WriteString(boundary)
	s.prefixedInline[first] = boundary != ""
	s.inlineStack = append(s.inlineStack, inlineFrame{owner: ast.NoNode, kind: firstNode.Kind(), marker: delimiter[0], merged: true})
	defer func() { s.inlineStack = s.inlineStack[:len(s.inlineStack)-1] }()
	for current := first; ; current = s.document.Node(current).NextSibling() {
		currentNode := s.document.Node(current)
		s.inlineStack[len(s.inlineStack)-1].hasPreceding = current != first
		s.inlineStack[len(s.inlineStack)-1].hasFollowing = current != last && !s.formattingGroupRemainderOnlyControls(current, last)
		s.inlineStack[len(s.inlineStack)-1].followingControl = current != last && s.startsWithUnrepresentableControl(s.document.Node(currentNode.NextSibling()))
		if current != first && isFormattingKind(currentNode.Kind()) && s.onlyUnrepresentableControlContent(current) {
			previous := s.document.Node(current).PreviousSibling()
			for previous != ast.NoNode && !isFormattingKind(s.document.Node(previous).Kind()) {
				previous = s.document.Node(previous).PreviousSibling()
			}
			if previous != ast.NoNode {
				s.writeBeforeTrailingFormattingClosers(s.document.Node(previous), s.unrepresentableControlContent(current))
				if current == last {
					break
				}
				continue
			}
		}
		if currentNode.Kind() == ast.Text {
			if err := s.inline(current); err != nil {
				return err
			}
		} else {
			if current != first {
				s.prepareFormattingGroupMember(currentNode)
			}
			if err := s.inlines(current); err != nil {
				return err
			}
		}
		if current == last {
			break
		}
	}
	s.stabilizeClosingBoundary(s.document.Node(last), rune(delimiter[0]))
	s.output.WriteString(delimiter)
	return nil
}

func (s *renderState) formattingGroupRemainderOnlyControls(current, last ast.NodeID) bool {
	for next := s.document.Node(current).NextSibling(); ; next = s.document.Node(next).NextSibling() {
		if !s.onlyUnrepresentableControlContent(next) {
			return false
		}
		if next == last {
			return true
		}
	}
}

func (s *renderState) onlyUnrepresentableControlContent(parent ast.NodeID) bool {
	hasContent := false
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		switch child.Kind() { //nolint:exhaustive // All non-text containers are rejected by default.
		case ast.Text:
			if !onlyUnrepresentableControls(child.Text()) {
				return false
			}
			hasContent = true
		case ast.Emphasis, ast.Strong:
			if !s.onlyUnrepresentableControlContent(childID) {
				return false
			}
			hasContent = true
		default:
			return false
		}
	}
	return hasContent
}

func (s *renderState) unrepresentableControlContent(parent ast.NodeID) string {
	var content strings.Builder
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		if child.Kind() == ast.Text {
			content.WriteString(child.Text())
			continue
		}
		content.WriteString(s.unrepresentableControlContent(childID))
	}
	return content.String()
}

func (s *renderState) formattingGroupHasEmphasisDescendant(first, last ast.NodeID) bool {
	for current := first; ; current = s.document.Node(current).NextSibling() {
		node := s.document.Node(current)
		if isFormattingKind(node.Kind()) && s.hasEmphasisDescendant(node) {
			return true
		}
		if current == last {
			return false
		}
	}
}

func (s *renderState) stabilizeFormattingGroupOpeningBoundary(first, last ast.NodeID, marker byte) {
	firstNode := s.document.Node(first)
	contentID := firstNode.FirstChild()
	if contentID == ast.NoNode || contentID != firstNode.LastChild() {
		return
	}
	content := s.document.Node(contentID)
	if isEmphasisKind(firstNode.Kind()) && isEmphasisKind(content.Kind()) {
		childMarker := s.effectiveInlineDelimiterMarker(content)
		if childMarker[0] == marker {
			childMarker = alternateInlineDelimiterMarker(childMarker)
		}
		if childMarker[0] != marker || marker == '_' {
			s.protectTrailingOutputRune()
		}
		return
	}
	if content.Kind() != ast.Text || utf8.RuneCountInString(content.Text()) != 1 {
		return
	}
	leading, _ := utf8.DecodeRuneInString(content.Text())
	if !isEntityBoundaryRune(leading) || !s.laterFormattingGroupMemberNeedsEntity(first, last) {
		return
	}
	s.protectTrailingOutputRune()
}

func (s *renderState) protectTrailingOutputRune() {
	output := s.output.String()
	boundary := len(output)
	for boundary > 0 {
		current, size := utf8.DecodeLastRuneInString(output[:boundary])
		if !unicode.IsControl(current) || numericEntityRoundTrips(current) {
			break
		}
		boundary -= size
	}
	previous, size := utf8.DecodeLastRuneInString(output[:boundary])
	if !isEntityBoundaryRune(previous) {
		return
	}
	s.output.Reset()
	s.output.WriteString(output[:boundary-size])
	s.writeNumericEntity(previous)
	s.output.WriteString(output[boundary:])
}

func (s *renderState) laterFormattingGroupMemberNeedsEntity(first, last ast.NodeID) bool {
	for current := s.document.Node(first).NextSibling(); ; current = s.document.Node(current).NextSibling() {
		node := s.document.Node(current)
		firstChildID := node.FirstChild()
		if firstChildID != ast.NoNode {
			firstChild := s.document.Node(firstChildID)
			if isFormattingKind(firstChild.Kind()) && !s.startsWithWordLikeText(firstChild) {
				return true
			}
		}
		if current == last {
			return false
		}
	}
}

func (s *renderState) prepareFormattingGroupMember(node ast.Node) {
	output := s.output.String()
	if output == "" {
		return
	}
	firstID := node.FirstChild()
	if firstID == ast.NoNode {
		return
	}
	first := s.document.Node(firstID)
	if (first.Kind() == ast.Emphasis || first.Kind() == ast.Strong) && s.hasDuplicateRecoveredLayer(first) {
		s.prepareCollapsedFormattingGroupMember(first)
		return
	}
	if isFormattingKind(first.Kind()) && !s.startsWithWordLikeText(first) {
		last, size := utf8.DecodeLastRuneInString(output)
		if isEntityBoundaryRune(last) {
			prefix := output[:len(output)-size]
			s.output.Reset()
			s.output.WriteString(prefix)
			s.writeNumericEntity(last)
		}
		return
	}
	if !endsWithUnescapedFormattingDelimiter(output) {
		return
	}
	if first.Kind() != ast.Text || first.Text() == "" {
		return
	}
	current, size := utf8.DecodeRuneInString(first.Text())
	if !isEntityBoundaryRune(current) {
		return
	}
	s.writeNumericEntity(current)
	s.skipText, s.skipByte = firstID, size
}

func (s *renderState) prepareCollapsedFormattingGroupMember(node ast.Node) {
	for (node.Kind() == ast.Emphasis || node.Kind() == ast.Strong) && s.hasDuplicateRecoveredLayer(node) {
		childID := node.FirstChild()
		if childID == ast.NoNode {
			return
		}
		node = s.document.Node(childID)
	}
	if node.Kind() != ast.Text || node.Text() == "" || !endsWithUnescapedFormattingDelimiter(s.output.String()) {
		return
	}
	current, size := utf8.DecodeRuneInString(node.Text())
	if !isEntityBoundaryRune(current) {
		return
	}
	s.writeNumericEntity(current)
	s.skipText, s.skipByte = node.ID(), size
}

func endsWithUnescapedFormattingDelimiter(output string) bool {
	if output == "" || !strings.ContainsRune("*_~", rune(output[len(output)-1])) {
		return false
	}
	backslashes := 0
	for index := len(output) - 2; index >= 0 && output[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 0
}

func (s *renderState) inline(id ast.NodeID) error { //nolint:gocyclo // The switch is the normalized inline grammar table.
	if err := s.ctx.Err(); err != nil {
		return err
	}
	node := s.document.Node(id)
	switch node.Kind() {
	case ast.Text:
		text := node.Text()
		if s.skipText == id {
			text = text[min(s.skipByte, len(text)):]
			s.skipText, s.skipByte = ast.NoNode, 0
		}
		s.writeText(node, text)
	case ast.SoftBreak:
		s.output.WriteByte('\n')
	case ast.HardBreak:
		s.output.WriteString("\\\n")
	case ast.CodeSpan:
		delimiter := strings.Repeat("`", longestRun(node.Text(), '`')+1)
		content := node.Text()
		// CommonMark strips one padding space only when the code is not made
		// entirely of spaces. Padding all-space content would therefore become
		// part of the value and grow on every normalization pass.
		spacePadding := strings.HasPrefix(content, " ") && strings.HasSuffix(content, " ") && strings.Trim(content, " ") != ""
		if strings.HasPrefix(content, "`") || strings.HasSuffix(content, "`") || spacePadding {
			content = " " + content + " "
		}
		s.output.WriteString(delimiter + content + delimiter)
	case ast.Emphasis:
		if s.hasDuplicateRecoveredLayer(node) {
			s.prepareCollapsedFormatting(node)
			return s.inlines(id)
		}
		delimiter := s.inlineDelimiter(node, 1)
		return s.inlineContainer(id, delimiter, delimiter)
	case ast.Strong:
		if s.hasDuplicateRecoveredLayer(node) {
			s.prepareCollapsedFormatting(node)
			return s.inlines(id)
		}
		delimiter := s.inlineDelimiter(node, 2)
		return s.inlineContainer(id, delimiter, delimiter)
	case ast.Strikethrough:
		if !s.renderer.profile.Has(profile.Strikethrough) {
			return fmt.Errorf("markdown: target profile %s does not support strikethrough", s.renderer.profile.ID())
		}
		if s.hasStrikethroughAncestor(node) {
			// Repeating deletion does not add semantics, while adjacent tilde
			// closers are ambiguous to Markdown delimiter parsers. Normalize any
			// nested deletion to its ancestor's single formatting layer.
			s.prepareCollapsedFormatting(node)
			return s.inlines(id)
		}
		delimiter := "~~"
		if node.Flags()&ast.StrikethroughSingleDelimiter != 0 {
			delimiter = "~"
		}
		return s.inlineContainer(id, delimiter, delimiter)
	case ast.Link:
		s.output.WriteByte('[')
		if err := s.inlines(id); err != nil {
			return err
		}
		s.output.WriteString("](" + escapeDestination(node.Destination()))
		if node.Title() != "" {
			s.output.WriteString(` "` + strings.ReplaceAll(node.Title(), `"`, `\"`) + `"`)
		}
		s.output.WriteByte(')')
	case ast.Image:
		s.output.WriteString("![")
		if err := s.inlines(id); err != nil {
			return err
		}
		s.output.WriteString("](" + escapeDestination(node.Destination()))
		if node.Title() != "" {
			s.output.WriteString(` "` + strings.ReplaceAll(node.Title(), `"`, `\"`) + `"`)
		}
		s.output.WriteByte(')')
	case ast.Autolink:
		if address, ok := strings.CutPrefix(node.Destination(), "mailto:"); ok && s.renderer.profile.Has(profile.ExtendedAutolinks) && !validAngleEmail(address) {
			if !validBareExtendedEmail(address) {
				// Extended email recognition may trim trailing punctuation from a
				// larger source token. If the resulting destination cannot stand on
				// its own as a bare autolink, retain the link with explicit syntax.
				s.output.WriteByte('[')
				s.output.WriteString(escapeText(address))
				s.output.WriteString("](" + escapeDestination(node.Destination()) + ")")
				break
			}
			if previousID := node.PreviousSibling(); previousID != ast.NoNode {
				previous := s.document.Node(previousID)
				previousRune, _ := utf8.DecodeLastRuneInString(previous.Text())
				if previous.Kind() != ast.Text || !unicode.IsSpace(previousRune) {
					s.output.WriteString(escapeText(address))
					break
				}
			}
			s.output.WriteString(address)
			break
		}
		if s.renderer.profile.Has(profile.ExtendedAutolinks) && strings.ContainsAny(node.Destination(), "<>") {
			s.output.WriteString(node.Destination())
			break
		}
		s.output.WriteByte('<')
		s.output.WriteString(strings.TrimPrefix(node.Destination(), "mailto:"))
		s.output.WriteByte('>')
	case ast.RawHTML:
		s.output.WriteString(node.Text())
	case ast.TaskCheck:
		if node.Flags()&ast.TaskChecked != 0 {
			s.output.WriteString("[x] ")
		} else {
			s.output.WriteString("[ ] ")
		}
	case ast.TableCell:
		return s.inlines(id)
	case ast.Invalid, ast.DocumentKind, ast.Paragraph, ast.Heading, ast.BlockQuote, ast.List, ast.ListItem, ast.ThematicBreak, ast.CodeBlock, ast.HTMLBlock, ast.Table, ast.TableHead, ast.TableBody, ast.TableRow:
		return fmt.Errorf("markdown: unexpected inline node %s", node.Kind())
	case ast.Custom:
		return s.custom(id, func(parent ast.NodeID) error { return s.inlines(parent) })
	default:
		return fmt.Errorf("markdown: unsupported inline node %s", node.Kind())
	}
	return nil
}

func (s *renderState) prepareCollapsedFormatting(node ast.Node) {
	previousID := node.PreviousSibling()
	firstID := node.FirstChild()
	if previousID == ast.NoNode || firstID == ast.NoNode || !isFormattingKind(s.document.Node(previousID).Kind()) {
		return
	}
	first := s.document.Node(firstID)
	if first.Kind() != ast.Text || first.Text() == "" || !endsWithUnescapedFormattingDelimiter(s.output.String()) {
		return
	}
	current, size := utf8.DecodeRuneInString(first.Text())
	if !isEntityBoundaryRune(current) {
		return
	}
	s.writeNumericEntity(current)
	s.skipText, s.skipByte = firstID, size
}

func (s *renderState) hasDuplicateRecoveredLayer(node ast.Node) bool {
	recovered := node.Flags()&ast.InlineRecoveredDelimiter != 0
	for parentID := node.Parent(); parentID != ast.NoNode; parentID = s.document.Node(parentID).Parent() {
		parent := s.document.Node(parentID)
		if parent.Kind() == node.Kind() && (recovered || parent.Flags()&ast.InlineRecoveredDelimiter != 0) {
			return true
		}
	}
	return false
}

func (s *renderState) hasStrikethroughAncestor(node ast.Node) bool {
	for parentID := node.Parent(); parentID != ast.NoNode; parentID = s.document.Node(parentID).Parent() {
		if s.document.Node(parentID).Kind() == ast.Strikethrough {
			return true
		}
	}
	return false
}

func (s *renderState) custom(id ast.NodeID, children func(ast.NodeID) error) error {
	node := s.document.Node(id)
	handler, ok := s.renderer.handlers.Handler(node.CustomKind())
	if !ok {
		return fmt.Errorf("markdown: no handler for custom node %q", node.CustomKind())
	}
	return baserenderer.RenderCustom(handler, baserenderer.ExtensionContext{
		RenderContext: s.ctx, Output: &s.output, AST: s.document, Children: children,
	}, id)
}

func (s *renderState) inlineContainer(id ast.NodeID, open, close string) error {
	if !s.hasRenderableInlineContent(id) {
		return nil
	}
	s.inlineStack = append(s.inlineStack, inlineFrame{owner: id, kind: s.document.Node(id).Kind(), marker: open[0]})
	defer func() { s.inlineStack = s.inlineStack[:len(s.inlineStack)-1] }()
	s.stabilizeInlineOpeningBoundary(s.document.Node(id))
	boundary := s.takeTrailingUnrepresentableControls()
	s.output.WriteString(open)
	s.output.WriteString(boundary)
	s.prefixedInline[id] = boundary != ""
	if err := s.inlines(id); err != nil {
		return err
	}
	s.stabilizeClosingBoundary(s.document.Node(id), rune(close[0]))
	s.output.WriteString(close)
	return nil
}

func (s *renderState) stabilizeInlineOpeningBoundary(node ast.Node) {
	if len(s.inlineStack) == 0 || !s.inlineOpeningNeedsProtection(node, s.inlineStack[len(s.inlineStack)-1].marker) {
		return
	}
	s.protectTrailingOutputRune()
}

func (s *renderState) inlineOpeningNeedsProtection(node ast.Node, marker byte) bool {
	firstID := node.FirstChild()
	if firstID == ast.NoNode {
		return false
	}
	first := s.document.Node(firstID)
	movedControls := false
	for first.Kind() == ast.Text && onlyUnrepresentableControls(first.Text()) && first.NextSibling() != ast.NoNode {
		movedControls = true
		firstID = first.NextSibling()
		first = s.document.Node(firstID)
	}
	for (first.Kind() == ast.Emphasis || first.Kind() == ast.Strong) && s.hasDuplicateRecoveredLayer(first) {
		firstID = first.FirstChild()
		if firstID == ast.NoNode {
			return false
		}
		first = s.document.Node(firstID)
	}
	needsProtection := isFormattingKind(first.Kind())
	if needsProtection {
		childMarker := s.effectiveInlineDelimiterMarker(first)
		if isEmphasisKind(node.Kind()) && isEmphasisKind(first.Kind()) {
			onlyChild := node.FirstChild() == first.ID() && node.LastChild() == first.ID()
			switch {
			case first.Kind() == ast.Strong && node.Kind() == ast.Emphasis:
				if onlyChild && !s.hasEmphasisDescendant(first) {
					childMarker = string(marker)
				} else if childMarker[0] == marker {
					childMarker = alternateInlineDelimiterMarker(childMarker)
				}
			case first.Kind() == ast.Emphasis && childMarker[0] == marker:
				childMarker = alternateInlineDelimiterMarker(childMarker)
			}
		}
		if movedControls && isEmphasisKind(node.Kind()) && isEmphasisKind(first.Kind()) && s.onlyChildAfterMovingControls(first) && !s.parentHasFollowingFormattingPeer(first) {
			childMarker = string(marker)
		}
		needsProtection = childMarker[0] != marker || marker == '_'
	}
	if first.Kind() == ast.Text {
		needsProtection = utf8.RuneCountInString(first.Text()) == 1 && s.needsTrailingEntity(first, first.Text())
	}
	return needsProtection
}

func (s *renderState) takeTrailingUnrepresentableControls() string {
	output := s.output.String()
	boundary := len(output)
	for boundary > 0 {
		current, size := utf8.DecodeLastRuneInString(output[:boundary])
		if !unicode.IsControl(current) || numericEntityRoundTrips(current) {
			break
		}
		boundary -= size
	}
	if boundary == len(output) {
		return ""
	}
	prefix, controls := output[:boundary], output[boundary:]
	s.output.Reset()
	s.output.WriteString(prefix)
	return controls
}

func (s *renderState) hasRenderableInlineContent(parent ast.NodeID) bool {
	if cached := s.inlineContent[parent]; cached != 0 {
		return cached == 2
	}
	for childID := s.document.Node(parent).FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		switch child.Kind() { //nolint:exhaustive // Every other inline kind always has renderable output.
		case ast.Text:
			if child.Text() != "" {
				s.inlineContent[parent] = 2
				return true
			}
		case ast.Emphasis, ast.Strong, ast.Strikethrough:
			if s.hasRenderableInlineContent(childID) {
				s.inlineContent[parent] = 2
				return true
			}
		default:
			s.inlineContent[parent] = 2
			return true
		}
	}
	s.inlineContent[parent] = 1
	return false
}

func (s *renderState) stabilizeClosingBoundary(node ast.Node, marker rune) {
	nextID := node.NextSibling()
	if nextID == ast.NoNode {
		return
	}
	next := s.document.Node(nextID)
	if next.Kind() != ast.Text || next.Text() == "" {
		return
	}
	previousRune, _ := utf8.DecodeLastRuneInString(s.output.String())
	nextRune, _ := utf8.DecodeRuneInString(next.Text())
	if delimiterCanClose(marker, previousRune, nextRune) || numericEntityRoundTrips(nextRune) {
		return
	}
	if !unicode.IsControl(nextRune) {
		return
	}
	// Markdown has no lossless character-reference spelling for NUL and some
	// HTML5 C1 controls. If such a rune would invalidate the closer, include
	// only that invisible boundary rune in the formatting and leave the rest
	// of the following Text node for normal rendering.
	boundaryBytes := 0
	for boundaryBytes < len(next.Text()) {
		current, size := utf8.DecodeRuneInString(next.Text()[boundaryBytes:])
		if !unicode.IsControl(current) || numericEntityRoundTrips(current) {
			break
		}
		boundaryBytes += size
	}
	s.writeBeforeTrailingFormattingClosers(node, next.Text()[:boundaryBytes])
	s.skipText, s.skipByte = nextID, boundaryBytes
}

func (s *renderState) writeBeforeTrailingFormattingClosers(node ast.Node, value string) {
	lastChild := node.LastChild()
	if lastChild == ast.NoNode || !isFormattingKind(s.document.Node(lastChild).Kind()) {
		s.output.WriteString(value)
		return
	}
	output := s.output.String()
	boundary := len(output)
	for boundary > 0 && strings.ContainsRune("*_~", rune(output[boundary-1])) {
		boundary--
	}
	prefix, suffix := output[:boundary], output[boundary:]
	s.output.Reset()
	s.output.WriteString(prefix)
	s.output.WriteString(value)
	s.output.WriteString(suffix)
}

func isFormattingKind(kind ast.Kind) bool {
	return kind == ast.Emphasis || kind == ast.Strong || kind == ast.Strikethrough
}

func delimiterCanClose(marker, previous, next rune) bool {
	previousWhitespace := unicode.IsSpace(previous)
	nextWhitespace := unicode.IsSpace(next)
	previousPunctuation := unicode.IsPunct(previous) || unicode.IsSymbol(previous)
	nextPunctuation := unicode.IsPunct(next) || unicode.IsSymbol(next)
	leftFlanking := !nextWhitespace && (!nextPunctuation || previousWhitespace || previousPunctuation)
	rightFlanking := !previousWhitespace && (!previousPunctuation || nextWhitespace || nextPunctuation)
	if marker == '_' {
		return rightFlanking && (!leftFlanking || nextPunctuation)
	}
	return rightFlanking
}

func (s *renderState) inlineDelimiter(node ast.Node, length int) string {
	marker := s.effectiveInlineDelimiterMarker(node)
	adjustedForParent := false
	if len(s.inlineStack) != 0 {
		parent := s.inlineStack[len(s.inlineStack)-1]
		astParent := s.document.Node(node.Parent())
		onlyChild := astParent.FirstChild() == node.ID() && astParent.LastChild() == node.ID()
		switch {
		case node.Kind() == ast.Strong && parent.kind == ast.Emphasis:
			combine := onlyChild && !parent.hasPreceding && !parent.hasFollowing
			if combine {
				marker = string(parent.marker)
			} else if marker[0] == parent.marker {
				marker = alternateInlineDelimiterMarker(marker)
			}
		case node.Kind() == ast.Emphasis && parent.kind == ast.Emphasis:
			if marker[0] == parent.marker {
				marker = alternateInlineDelimiterMarker(marker)
			}
			adjustedForParent = true
		case node.Kind() == ast.Emphasis && marker[0] == parent.marker && !onlyChild:
			marker = alternateInlineDelimiterMarker(marker)
			adjustedForParent = true
		}
	}
	if previousID := s.previousFormattingSibling(node); previousID != ast.NoNode {
		output := s.output.String()
		boundary := len(output)
		for boundary > 0 {
			current, size := utf8.DecodeLastRuneInString(output[:boundary])
			if !unicode.IsControl(current) || numericEntityRoundTrips(current) {
				break
			}
			boundary -= size
		}
		if endsWithUnescapedFormattingDelimiter(output[:boundary]) && output[boundary-1] == marker[0] {
			marker = alternateInlineDelimiterMarker(marker)
		}
	}
	if node.Kind() == ast.Emphasis && !adjustedForParent {
		for index := len(s.inlineStack) - 1; index >= 0; index-- {
			ancestor := s.inlineStack[index]
			if ancestor.kind != ast.Emphasis || marker[0] != ancestor.marker {
				continue
			}
			immediateParent := index == len(s.inlineStack)-1
			if immediateParent || !s.startsWithWordLikeText(node) {
				marker = alternateInlineDelimiterMarker(marker)
				break
			}
		}
	}
	if marker == "_" && s.followedByUnrepresentableControl(node) && s.previousFormattingSibling(node) == ast.NoNode {
		marker = "*"
	}
	if marker == "_" {
		output := s.output.String()
		boundary := len(output)
		for boundary > 0 {
			current, size := utf8.DecodeLastRuneInString(output[:boundary])
			if !unicode.IsControl(current) || numericEntityRoundTrips(current) {
				break
			}
			boundary -= size
		}
		if boundary != len(output) {
			parentID := node.Parent()
			if parentID != ast.NoNode {
				parent := s.document.Node(parentID)
				if isEmphasisKind(parent.Kind()) && s.previousFormattingSibling(node) == ast.NoNode && !s.parentHasFollowingFormattingPeer(node) && s.canReuseParentMarkerAcrossControls(node) && (node.Flags()&ast.InlineRecoveredDelimiter != 0 || s.onlyChildAfterMovingControls(node)) {
					parentMarker := s.effectiveInlineDelimiterMarker(parent)
					if len(s.inlineStack) != 0 && s.inlineStack[len(s.inlineStack)-1].kind == parent.Kind() {
						parentMarker = string(s.inlineStack[len(s.inlineStack)-1].marker)
					}
					if strings.HasSuffix(output[:boundary], parentMarker) {
						marker = parentMarker
					}
				}
			}
		}
		previous, _ := utf8.DecodeLastRuneInString(output[:boundary])
		if marker == "_" && !unicode.IsSpace(previous) && !unicode.IsPunct(previous) && !unicode.IsSymbol(previous) && !s.inlineOpeningNeedsProtection(node, marker[0]) {
			// Underscores cannot open inside a word-like boundary. Controls that
			// cannot round-trip through a character reference and ordinary word
			// runes must stay literal; an asterisk is valid at either boundary.
			marker = "*"
		}
	}
	return strings.Repeat(marker, length)
}

func (s *renderState) canReuseParentMarkerAcrossControls(node ast.Node) bool {
	if len(s.inlineStack) == 0 {
		return true
	}
	parent := s.inlineStack[len(s.inlineStack)-1]
	if parent.hasFollowing {
		return false
	}
	return !parent.hasPreceding || s.onlyChildAfterMovingControls(node)
}

func (s *renderState) parentHasFollowingFormattingPeer(node ast.Node) bool {
	parentID := node.Parent()
	if parentID == ast.NoNode {
		return false
	}
	for nextID := s.document.Node(parentID).NextSibling(); nextID != ast.NoNode; nextID = s.document.Node(nextID).NextSibling() {
		next := s.document.Node(nextID)
		if next.Kind() == node.Kind() {
			return true
		}
		if next.Kind() != ast.Text || !onlyUnrepresentableControls(next.Text()) {
			return false
		}
	}
	return false
}

func (s *renderState) followedByUnrepresentableControl(node ast.Node) bool {
	if next := node.NextSibling(); next != ast.NoNode && s.startsWithUnrepresentableControl(s.document.Node(next)) {
		return true
	}
	return len(s.inlineStack) != 0 && s.inlineStack[len(s.inlineStack)-1].followingControl
}

func (s *renderState) onlyChildAfterMovingControls(node ast.Node) bool {
	if node.NextSibling() != ast.NoNode {
		return false
	}
	for previousID := node.PreviousSibling(); previousID != ast.NoNode; previousID = s.document.Node(previousID).PreviousSibling() {
		previous := s.document.Node(previousID)
		if previous.Kind() != ast.Text || !onlyUnrepresentableControls(previous.Text()) {
			return false
		}
	}
	return true
}

func (s *renderState) effectiveInlineDelimiterMarker(node ast.Node) string {
	if cached := s.inlineMarkers[node.ID()]; cached != 0 {
		return string(cached)
	}
	marker := inlineDelimiterMarker(node)
	if parentID := node.Parent(); parentID != ast.NoNode {
		parent := s.document.Node(parentID)
		if parent.Kind() == ast.Emphasis || parent.Kind() == ast.Strong {
			parentMarker := s.effectiveInlineDelimiterMarker(parent)
			onlyChild := parent.FirstChild() == node.ID() && parent.LastChild() == node.ID()
			combineRun := onlyChild && !s.parentHasFollowingFormattingPeer(node) && (node.Kind() != ast.Emphasis || !s.combinedRunHasEmphasis(parent) && !s.hasEmphasisDescendant(node))
			if combineRun {
				marker = parentMarker
			} else if marker == parentMarker {
				marker = alternateInlineDelimiterMarker(marker)
			}
		}
	}
	if previousID := s.previousFormattingSibling(node); previousID != ast.NoNode {
		previous := s.document.Node(previousID)
		previousMarker := s.effectiveInlineDelimiterMarker(previous)
		if previousMarker == marker {
			marker = alternateInlineDelimiterMarker(marker)
		}
	}
	s.inlineMarkers[node.ID()] = marker[0]
	return marker
}

func (s *renderState) hasEmphasisDescendant(node ast.Node) bool {
	for childID := node.FirstChild(); childID != ast.NoNode; childID = s.document.Node(childID).NextSibling() {
		child := s.document.Node(childID)
		if (child.Kind() == ast.Emphasis && !s.hasDuplicateRecoveredLayer(child)) || s.hasEmphasisDescendant(child) {
			return true
		}
	}
	return false
}

func (s *renderState) previousFormattingSibling(node ast.Node) ast.NodeID {
	for previousID := node.PreviousSibling(); previousID != ast.NoNode; previousID = s.document.Node(previousID).PreviousSibling() {
		previous := s.document.Node(previousID)
		if isEmphasisKind(previous.Kind()) {
			return previousID
		}
		if previous.Kind() != ast.Text || !onlyUnrepresentableControls(previous.Text()) {
			return ast.NoNode
		}
	}
	return ast.NoNode
}

func onlyUnrepresentableControls(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range value {
		if !unicode.IsControl(current) || numericEntityRoundTrips(current) {
			return false
		}
	}
	return true
}

func (s *renderState) combinedRunHasEmphasis(node ast.Node) bool {
	marker := s.effectiveInlineDelimiterMarker(node)
	for {
		if node.Kind() == ast.Emphasis {
			return true
		}
		parentID := node.Parent()
		if parentID == ast.NoNode {
			return false
		}
		parent := s.document.Node(parentID)
		if !isEmphasisKind(parent.Kind()) || parent.FirstChild() != node.ID() || parent.LastChild() != node.ID() || s.effectiveInlineDelimiterMarker(parent) != marker {
			return false
		}
		node = parent
	}
}

func isEmphasisKind(kind ast.Kind) bool {
	return kind == ast.Emphasis || kind == ast.Strong
}

func inlineDelimiterMarker(node ast.Node) string {
	if node.Flags()&ast.InlineDelimiterUnderscore != 0 {
		return "_"
	}
	return "*"
}

func alternateInlineDelimiterMarker(marker string) string {
	if marker == "*" {
		return "_"
	}
	return "*"
}

func (s *renderState) writeText(node ast.Node, text string) {
	start, end := 0, len(text)
	parent := s.document.Node(node.Parent())
	formattedParent := parent.Kind() == ast.Emphasis || parent.Kind() == ast.Strong || parent.Kind() == ast.Strikethrough
	if formattedParent && !s.prefixedInline[parent.ID()] && s.atRenderedFormattingLeadingEdge(parent, node) {
		for start < end {
			current, size := utf8.DecodeRuneInString(text[start:end])
			if !unicode.IsSpace(current) {
				break
			}
			s.writeNumericEntity(current)
			start += size
		}
	}
	trailingStart := end
	if formattedParent && s.atRenderedFormattingTrailingEdge(parent, node) {
		for trailingStart > start {
			current, size := utf8.DecodeLastRuneInString(text[start:trailingStart])
			if !unicode.IsSpace(current) {
				break
			}
			trailingStart -= size
		}
	}
	middle := text[start:trailingStart]
	if s.needsLeadingEntity(node, middle) {
		first, size := utf8.DecodeRuneInString(middle)
		s.writeNumericEntity(first)
		middle = middle[size:]
	}
	trailingEntity := rune(0)
	hasTrailingEntity := false
	if s.needsTrailingEntity(node, middle) && !s.mustKeepOnlyFormattingRuneLiteral(node, parent, middle) {
		var size int
		trailingEntity, size = utf8.DecodeLastRuneInString(middle)
		middle = middle[:len(middle)-size]
		hasTrailingEntity = true
	}
	s.output.WriteString(escapeText(middle))
	if hasTrailingEntity {
		s.writeNumericEntity(trailingEntity)
	}
	for trailingStart < end {
		current, size := utf8.DecodeRuneInString(text[trailingStart:end])
		s.writeNumericEntity(current)
		trailingStart += size
	}
}

func (s *renderState) mustKeepOnlyFormattingRuneLiteral(node, parent ast.Node, text string) bool {
	if text == "" || !s.atFormattingLeadingEdge(parent, node) {
		return false
	}
	remaining := text
	hasControlBoundary := false
	for remaining != "" {
		current, size := utf8.DecodeRuneInString(remaining)
		if !unicode.IsControl(current) || numericEntityRoundTrips(current) {
			break
		}
		hasControlBoundary = true
		remaining = remaining[size:]
	}
	if utf8.RuneCountInString(remaining) != 1 {
		return false
	}
	if hasControlBoundary {
		return true
	}
	previousID := parent.PreviousSibling()
	if previousID == ast.NoNode {
		return false
	}
	previous := s.document.Node(previousID)
	if previous.Kind() != ast.Text || previous.Text() == "" {
		return false
	}
	boundary, _ := utf8.DecodeLastRuneInString(previous.Text())
	return unicode.IsControl(boundary) && !numericEntityRoundTrips(boundary)
}

func (s *renderState) atFormattingLeadingEdge(parent, node ast.Node) bool {
	for siblingID := parent.FirstChild(); siblingID != node.ID(); siblingID = s.document.Node(siblingID).NextSibling() {
		sibling := s.document.Node(siblingID)
		if sibling.Kind() != ast.Text || strings.TrimFunc(sibling.Text(), unicode.IsSpace) != "" {
			return false
		}
	}
	return true
}

func (s *renderState) atRenderedFormattingLeadingEdge(parent, node ast.Node) bool {
	if !s.atFormattingLeadingEdge(parent, node) {
		return false
	}
	if len(s.inlineStack) == 0 {
		return true
	}
	frame := s.inlineStack[len(s.inlineStack)-1]
	return !frame.merged || frame.kind != parent.Kind() || !frame.hasPreceding
}

func (s *renderState) atFormattingTrailingEdge(parent, node ast.Node) bool {
	for siblingID := node.NextSibling(); siblingID != ast.NoNode; siblingID = s.document.Node(siblingID).NextSibling() {
		sibling := s.document.Node(siblingID)
		if sibling.Kind() != ast.Text || strings.TrimFunc(sibling.Text(), unicode.IsSpace) != "" {
			return false
		}
	}
	return true
}

func (s *renderState) atRenderedFormattingTrailingEdge(parent, node ast.Node) bool {
	if !s.atFormattingTrailingEdge(parent, node) {
		return false
	}
	if len(s.inlineStack) == 0 {
		return true
	}
	frame := s.inlineStack[len(s.inlineStack)-1]
	return !frame.merged || frame.kind != parent.Kind() || !frame.hasFollowing
}

func (s *renderState) writeNumericEntity(current rune) {
	s.output.WriteString("&#" + strconv.Itoa(int(current)) + ";")
}

func (s *renderState) needsLeadingEntity(node ast.Node, text string) bool {
	if text == "" || node.PreviousSibling() == ast.NoNode {
		return false
	}
	first, _ := utf8.DecodeRuneInString(text)
	previous := s.document.Node(node.PreviousSibling())
	if previous.Kind() == ast.Autolink && strings.ContainsAny(previous.Destination(), "<>") && strings.ContainsRune(`\`+"`*_{}[]<>#+-.:!|~)", first) {
		return true
	}
	if !isEntityBoundaryRune(first) {
		return false
	}
	if s.isCollapsedFormatting(previous) && !s.collapsedFormattingEndsWithDelimiter(previous) {
		return false
	}
	return previous.Kind() == ast.Emphasis || previous.Kind() == ast.Strong || previous.Kind() == ast.Strikethrough
}

func (s *renderState) isCollapsedFormatting(node ast.Node) bool {
	return (isEmphasisKind(node.Kind()) && s.hasDuplicateRecoveredLayer(node)) ||
		(node.Kind() == ast.Strikethrough && s.hasStrikethroughAncestor(node))
}

func (s *renderState) collapsedFormattingEndsWithDelimiter(node ast.Node) bool {
	for childID := node.LastChild(); childID != ast.NoNode; childID = s.document.Node(childID).PreviousSibling() {
		child := s.document.Node(childID)
		if child.Kind() == ast.Text {
			if child.Text() != "" {
				return false
			}
			continue
		}
		if isFormattingKind(child.Kind()) {
			if ((child.Kind() == ast.Emphasis || child.Kind() == ast.Strong) && s.hasDuplicateRecoveredLayer(child)) ||
				(child.Kind() == ast.Strikethrough && s.hasStrikethroughAncestor(child)) {
				return s.collapsedFormattingEndsWithDelimiter(child)
			}
			return s.hasRenderableInlineContent(childID)
		}
		return false
	}
	return false
}

func (s *renderState) needsTrailingEntity(node ast.Node, text string) bool {
	if text == "" || node.NextSibling() == ast.NoNode {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(text)
	if !isEntityBoundaryRune(last) {
		return false
	}
	next := s.document.Node(node.NextSibling())
	if s.isCollapsedFormatting(next) {
		return false
	}
	if isFormattingKind(next.Kind()) && s.startsWithUnrepresentableControl(next) && !s.asteriskFallbackViolatesRuleOfThree(next) {
		return false
	}
	if isFormattingKind(next.Kind()) && s.startsWithWordLikeText(next) && !s.asteriskFallbackViolatesRuleOfThree(next) {
		return false
	}
	return next.Kind() == ast.Emphasis || next.Kind() == ast.Strong || next.Kind() == ast.Strikethrough
}

func (s *renderState) asteriskFallbackViolatesRuleOfThree(node ast.Node) bool {
	if !isEmphasisKind(node.Kind()) || node.NextSibling() != ast.NoNode || s.effectiveInlineDelimiterMarker(node) != "_" {
		return false
	}
	parentRun := 0
	for index := len(s.inlineStack) - 1; index >= 0; index-- {
		frame := s.inlineStack[index]
		if !isEmphasisKind(frame.kind) || frame.marker != '*' {
			break
		}
		parentRun += delimiterLength(frame.kind)
	}
	if parentRun == 0 {
		return false
	}
	openingRun := delimiterLength(node.Kind())
	closingRun := parentRun + openingRun
	return (openingRun+closingRun)%3 == 0 && (openingRun%3 != 0 || closingRun%3 != 0)
}

func delimiterLength(kind ast.Kind) int {
	if kind == ast.Strong {
		return 2
	}
	return 1
}

func (s *renderState) startsWithWordLikeText(node ast.Node) bool {
	child := s.document.Node(node.FirstChild())
	for (child.Kind() == ast.Emphasis || child.Kind() == ast.Strong) && s.hasDuplicateRecoveredLayer(child) {
		child = s.document.Node(child.FirstChild())
	}
	if child.Kind() != ast.Text || child.Text() == "" {
		return false
	}
	if utf8.RuneCountInString(child.Text()) == 1 && s.needsTrailingEntity(child, child.Text()) && !s.mustKeepOnlyFormattingRuneLiteral(child, node, child.Text()) {
		return false
	}
	current, _ := utf8.DecodeRuneInString(child.Text())
	return !unicode.IsSpace(current) && !unicode.IsPunct(current) && !unicode.IsSymbol(current)
}

func (s *renderState) startsWithUnrepresentableControl(node ast.Node) bool {
	for isFormattingKind(node.Kind()) {
		childID := node.FirstChild()
		if childID == ast.NoNode {
			return false
		}
		node = s.document.Node(childID)
	}
	if node.Kind() != ast.Text || node.Text() == "" {
		return false
	}
	current, _ := utf8.DecodeRuneInString(node.Text())
	return unicode.IsControl(current) && !numericEntityRoundTrips(current)
}

func isEntityBoundaryRune(current rune) bool {
	if unicode.IsSpace(current) || unicode.IsPunct(current) || unicode.IsSymbol(current) {
		return false
	}
	// A numeric reference puts punctuation next to the delimiter while parsing,
	// then restores the original rune in the Text node. Some numeric references
	// (notably NUL and the HTML5 C1 replacements) do not decode to the rune that
	// named them, so leave those values literal rather than corrupting content.
	return numericEntityRoundTrips(current)
}

func numericEntityRoundTrips(current rune) bool {
	entity := "&#" + strconv.Itoa(int(current)) + ";"
	return html.UnescapeString(entity) == string(current)
}

func validAngleEmail(address string) bool {
	parsed, err := mail.ParseAddress(address)
	return err == nil && parsed.Address == address
}

func validBareExtendedEmail(address string) bool {
	position := 0
	for position < len(address) && (isASCIIAlphanumeric(address[position]) || strings.ContainsRune(".-_+", rune(address[position]))) {
		position++
	}
	if position == 0 || position >= len(address) || address[position] != '@' {
		return false
	}
	position++
	domainStart := position
	hasPeriod := false
	for position < len(address) && (isASCIIAlphanumeric(address[position]) || strings.ContainsRune(".-_", rune(address[position]))) {
		hasPeriod = hasPeriod || address[position] == '.'
		position++
	}
	return position == len(address) && position > domainStart && hasPeriod && address[position-1] != '.' && address[position-1] != '-' && address[position-1] != '_'
}

func isASCIIAlphanumeric(current byte) bool {
	return current >= '0' && current <= '9' || current >= 'A' && current <= 'Z' || current >= 'a' && current <= 'z'
}

func (s *renderState) renderList(list ast.NodeID, depth int) error {
	node := s.document.Node(list)
	ordered := node.Flags()&ast.ListOrdered != 0
	marker := s.listMarker(node, ordered)
	start, _ := node.Integers()
	index := 0
	for item := node.FirstChild(); item != ast.NoNode; item = s.document.Node(item).NextSibling() {
		prefix := marker + " "
		if ordered {
			prefix = strconv.Itoa(start+index) + marker + " "
		}
		content, err := s.renderItemToString(item)
		if err != nil {
			return err
		}
		lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
		for lineIndex, line := range lines {
			if line == "" && lineIndex != 0 {
				s.output.WriteByte('\n')
				continue
			}
			s.output.WriteString(strings.Repeat("  ", depth))
			if lineIndex == 0 {
				s.output.WriteString(prefix)
			} else {
				s.output.WriteString(strings.Repeat(" ", len(prefix)))
			}
			s.output.WriteString(line)
			s.output.WriteByte('\n')
		}
		index++
	}
	return nil
}

func (s *renderState) listMarker(node ast.Node, ordered bool) string {
	variant := 0
	for previousID := node.PreviousSibling(); previousID != ast.NoNode; previousID = s.document.Node(previousID).PreviousSibling() {
		previous := s.document.Node(previousID)
		if previous.Kind() != ast.List || (previous.Flags()&ast.ListOrdered != 0) != ordered {
			break
		}
		variant++
	}
	for parentID := node.Parent(); parentID != ast.NoNode; parentID = s.document.Node(parentID).Parent() {
		if s.document.Node(parentID).Kind() == ast.List {
			variant++
		}
	}
	if ordered {
		if variant%2 == 0 {
			return "."
		}
		return ")"
	}
	return []string{"-", "+", "*"}[variant%3]
}

func (s *renderState) renderItemToString(item ast.NodeID) (string, error) {
	nested := newRenderState(s.renderer, s.ctx, s.document)
	for child := s.document.Node(item).FirstChild(); child != ast.NoNode; child = s.document.Node(child).NextSibling() {
		kind := s.document.Node(child).Kind()
		if kind == ast.TaskCheck {
			if err := nested.inline(child); err != nil {
				return "", err
			}
			continue
		}
		if err := nested.block(child); err != nil {
			return "", err
		}
	}
	return strings.TrimRight(nested.output.String(), "\n"), nil
}

func (s *renderState) renderTable(table ast.NodeID) error {
	head := s.document.Node(table).FirstChild()
	if head == ast.NoNode || s.document.Node(head).Kind() != ast.TableHead {
		return fmt.Errorf("markdown: table has no header")
	}
	headerRow := s.document.Node(head).FirstChild()
	if headerRow == ast.NoNode {
		return fmt.Errorf("markdown: table header is empty")
	}
	if err := s.renderTableRow(headerRow); err != nil {
		return err
	}
	s.output.WriteByte('|')
	for cell := s.document.Node(headerRow).FirstChild(); cell != ast.NoNode; cell = s.document.Node(cell).NextSibling() {
		s.output.WriteString(" " + tableDelimiter(s.document.Node(cell).Flags()) + " |")
	}
	s.output.WriteByte('\n')
	body := s.document.Node(head).NextSibling()
	if body != ast.NoNode {
		for row := s.document.Node(body).FirstChild(); row != ast.NoNode; row = s.document.Node(row).NextSibling() {
			if err := s.renderTableRow(row); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *renderState) renderTableRow(row ast.NodeID) error {
	s.output.WriteByte('|')
	for cell := s.document.Node(row).FirstChild(); cell != ast.NoNode; cell = s.document.Node(cell).NextSibling() {
		nested := newRenderState(s.renderer, s.ctx, s.document)
		if err := nested.inlines(cell); err != nil {
			return err
		}
		content := strings.ReplaceAll(nested.output.String(), "|", "\\|")
		s.output.WriteString(" " + content + " |")
	}
	s.output.WriteByte('\n')
	return nil
}

func tableDelimiter(flags uint32) string {
	switch {
	case flags&ast.TableAlignCenter != 0:
		return ":---:"
	case flags&ast.TableAlignLeft != 0:
		return ":---"
	case flags&ast.TableAlignRight != 0:
		return "---:"
	default:
		return "---"
	}
}

func (s *renderState) renderBlocksToString(parent ast.NodeID) (string, error) {
	nested := newRenderState(s.renderer, s.ctx, s.document)
	if err := nested.blocks(parent); err != nil {
		return "", err
	}
	return nested.output.String(), nil
}

func (s *renderState) blankLine() {
	trimmed := strings.TrimRight(s.output.String(), " \t\n")
	s.output.Reset()
	s.output.WriteString(trimmed)
	if trimmed != "" {
		s.output.WriteString("\n\n")
	}
}

func escapeText(value string) string {
	var output strings.Builder
	output.Grow(len(value))
	for _, current := range value {
		if current == '\r' {
			output.WriteString("&#13;")
			continue
		}
		if strings.ContainsRune(`\`+"`*_{}[]<>#+-.:!|~)", current) {
			output.WriteByte('\\')
		}
		output.WriteRune(current)
	}
	return output.String()
}

func escapeDestination(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, ")", "\\)")
	return value
}

func longestRun(value string, marker byte) int {
	longest, current := 0, 0
	for index := 0; index < len(value); index++ {
		if value[index] == marker {
			current++
			longest = maxInt(longest, current)
		} else {
			current = 0
		}
	}
	return longest
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
