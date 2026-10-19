// Deliberately mismatching implementation fragment for the conformance example.
package turnstile

type Counter struct {
	passages uint64
}

// AcceptEntry represents an accepted entry, but deliberately fails to update passages.
func (c *Counter) AcceptEntry() {
}
