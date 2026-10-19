package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

func decodeJSON(data []byte) (any, error) {
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) || !utf8.Valid(data) {
		return nil, rule("utf8_invalid", "json")
	}
	s := jsonScanner{data: data}
	if err := s.validate(); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, rule("json_syntax", "json")
	}
	return value, nil
}

const (
	objectFirstKey byte = iota
	objectKey
	objectColon
	objectValue
	objectComma
	arrayFirstValue
	arrayValue
	arrayComma
)

// jsonScanner validates the complete root without recursion or a value tree.
// Only the first sixteen object scopes retain decoded names; once a semantic
// failure is known, the scanner continues solely to establish syntax precedence.
type jsonScanner struct {
	data     []byte
	pos      int
	states   []byte
	names    [16]map[string]bool
	semantic error
}

func (s *jsonScanner) reject(code string, offset int) error {
	return rule(code, fmt.Sprintf("byte:%d", offset))
}

func (s *jsonScanner) whitespace() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\t', '\r', '\n':
			s.pos++
		default:
			return
		}
	}
}

func (s *jsonScanner) close() {
	depth := len(s.states)
	if depth <= len(s.names) {
		s.names[depth-1] = nil
	}
	s.states = s.states[:depth-1]
	s.pos++
}

func (s *jsonScanner) validate() error {
	started := false
	for {
		s.whitespace()
		depth := len(s.states)
		if started && depth == 0 {
			break
		}
		if s.pos >= len(s.data) {
			return s.reject("json_syntax", s.pos)
		}
		b := s.data[s.pos]
		if depth != 0 {
			switch s.states[depth-1] {
			case objectFirstKey, objectKey:
				if b == '}' && s.states[depth-1] == objectFirstKey {
					s.close()
					continue
				}
				if b != '"' {
					return s.reject("json_syntax", s.pos)
				}
				start := s.pos
				if !s.stringToken() {
					return s.reject("json_syntax", s.pos)
				}
				if s.semantic == nil {
					var name string
					if err := json.Unmarshal(s.data[start:s.pos], &name); err != nil {
						return s.reject("json_syntax", start)
					}
					if s.names[depth-1][name] {
						s.semantic = s.reject("json_duplicate_name", start)
					} else {
						s.names[depth-1][name] = true
					}
				}
				s.states[depth-1] = objectColon
				continue
			case objectColon:
				if b != ':' {
					return s.reject("json_syntax", s.pos)
				}
				s.pos++
				s.states[depth-1] = objectValue
				continue
			case objectComma, arrayComma:
				object := s.states[depth-1] == objectComma
				if object && b == '}' || !object && b == ']' {
					s.close()
					continue
				}
				if b != ',' {
					return s.reject("json_syntax", s.pos)
				}
				s.pos++
				if object {
					s.states[depth-1] = objectKey
				} else {
					s.states[depth-1] = arrayValue
				}
				continue
			case arrayFirstValue:
				if b == ']' {
					s.close()
					continue
				}
			}
			if s.states[depth-1] == objectValue {
				s.states[depth-1] = objectComma
			} else {
				s.states[depth-1] = arrayComma
			}
		}
		started = true
		switch b {
		case '{', '[':
			if depth == 16 && s.semantic == nil {
				s.semantic = s.reject("json_depth", s.pos)
			}
			state := arrayFirstValue
			if b == '{' {
				state = objectFirstKey
				if s.semantic == nil {
					s.names[depth] = make(map[string]bool)
				}
			}
			s.states = append(s.states, state)
			s.pos++
		case '"':
			if !s.stringToken() {
				return s.reject("json_syntax", s.pos)
			}
		case 't', 'f', 'n':
			literal := "null"
			if b == 't' {
				literal = "true"
			}
			if b == 'f' {
				literal = "false"
			}
			if !bytes.HasPrefix(s.data[s.pos:], []byte(literal)) {
				return s.reject("json_syntax", s.pos)
			}
			s.pos += len(literal)
		default:
			if !s.numberToken() {
				return s.reject("json_syntax", s.pos)
			}
		}
	}
	if s.pos != len(s.data) {
		return s.reject("json_trailing_data", s.pos)
	}
	return s.semantic
}

func (s *jsonScanner) stringToken() bool {
	s.pos++
	for s.pos < len(s.data) {
		b := s.data[s.pos]
		s.pos++
		switch {
		case b == '"':
			return true
		case b < 0x20:
			return false
		case b == '\\':
			if s.pos >= len(s.data) {
				return false
			}
			escape := s.data[s.pos]
			s.pos++
			switch escape {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			case 'u':
				if len(s.data)-s.pos < 4 {
					return false
				}
				x := hex4(s.data[s.pos : s.pos+4])
				if x < 0 {
					return false
				}
				s.pos += 4
				if x >= 0xd800 && x <= 0xdbff {
					if len(s.data)-s.pos < 6 || s.data[s.pos] != '\\' || s.data[s.pos+1] != 'u' {
						return false
					}
					y := hex4(s.data[s.pos+2 : s.pos+6])
					if y < 0xdc00 || y > 0xdfff {
						return false
					}
					s.pos += 6
				} else if x >= 0xdc00 && x <= 0xdfff {
					return false
				}
			default:
				return false
			}
		}
	}
	return false
}

func hex4(b []byte) int {
	n := 0
	for _, c := range b {
		n *= 16
		switch {
		case c >= '0' && c <= '9':
			n += int(c - '0')
		case c >= 'a' && c <= 'f':
			n += int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n += int(c-'A') + 10
		default:
			return -1
		}
	}
	return n
}

func (s *jsonScanner) digit() bool {
	if s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
		s.pos++
		return true
	}
	return false
}

func (s *jsonScanner) numberToken() bool {
	if s.data[s.pos] == '-' {
		s.pos++
	}
	if s.pos >= len(s.data) {
		return false
	}
	if s.data[s.pos] == '0' {
		s.pos++
		if s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			return false
		}
	} else {
		if !s.digit() {
			return false
		}
		for s.digit() {
		}
	}
	if s.pos < len(s.data) && s.data[s.pos] == '.' {
		s.pos++
		if !s.digit() {
			return false
		}
		for s.digit() {
		}
	}
	if s.pos < len(s.data) && (s.data[s.pos] == 'e' || s.data[s.pos] == 'E') {
		s.pos++
		if s.pos < len(s.data) && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
			s.pos++
		}
		if !s.digit() {
			return false
		}
		for s.digit() {
		}
	}
	return true
}

func scanStructured(v any) error {
	switch value := v.(type) {
	case map[string]any:
		for name, child := range value {
			if secretMarker(name) {
				return rule("secret_indicator", "json")
			}
			if err := scanStructured(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := scanStructured(child); err != nil {
				return err
			}
		}
	case string:
		if secretValue(value) {
			return rule("secret_indicator", "json")
		}
	}
	return nil
}
