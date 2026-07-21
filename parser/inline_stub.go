package parser

// parseInlines is replaced by the delimiter-aware inline parser in the next
// implementation slice. Keeping the block parser independently buildable makes
// its source mapping and container behavior reviewable in isolation.
func (s *parseState) parseInlines() error { return nil }
