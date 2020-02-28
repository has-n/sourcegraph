package search

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type Node interface {
	String() string
}

type Parameter struct {
	Value string
}

type Op struct {
	Kind     string
	Children []Node
}

func (node Parameter) String() string {
	return node.Value
}

func (node Op) String() string {
	var result []string
	for _, child := range node.Children {
		result = append(result, child.String())
	}
	return fmt.Sprintf("(%s %s)", strings.ToLower(node.Kind), strings.Join(result, " "))
}

func isSpace(c byte) bool {
	return (c == ' ') || (c == '\n') || (c == '\r') || (c == '\t')
}

func skipSpace(buf []byte) int {
	for i, c := range buf {
		if !isSpace(c) {
			return i
		}
	}
	return len(buf)
}

type state struct {
	buf      []byte
	pos      int
	balanced int
}

func (s *state) done() bool {
	return s.pos >= len(s.buf)
}

func (s *state) peek(n int) (string, error) {
	if s.pos+n > len(s.buf) {
		return "", io.ErrShortBuffer
	}
	return string(s.buf[s.pos : s.pos+n]), nil
}

func (s *state) skipSpaces() error {
	if s.pos > len(s.buf) {
		return io.ErrShortBuffer
	}

	s.pos += skipSpace(s.buf[s.pos:])
	if s.pos > len(s.buf) {
		return io.ErrShortBuffer
	}
	return nil
}

func (s *state) match(keyword string) bool {
	v, err := s.peek(len(keyword))
	if err != nil {
		return false
	}
	return strings.ToLower(v) == keyword
}

func (s *state) expect(keyword string) bool {
	if !s.match(keyword) {
		return false
	}
	s.pos += len(keyword)
	return true
}

func (s *state) isKeyword() bool {
	return s.match("and") || s.match("or") || s.match("(") || s.match(")")
}

func (s *state) scanParameter() (string, error) {
	start := s.pos
	for {
		if s.isKeyword() {
			break
		}
		if s.done() {
			break
		}
		if isSpace(s.buf[s.pos]) {
			break
		}
		s.pos++
	}
	return string(s.buf[start:s.pos]), nil
}

func (s *state) parseParameterList() ([]Node, error) {
	var nodes []Node
	for {
		if err := s.skipSpaces(); err != nil {
			return nil, err
		}
		if s.done() {
			break
		}
		switch {
		case s.expect("("):
			s.balanced++
			result, err := s.parseOr()
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, result...)
		case s.expect(")"):
			s.balanced--
			if len(nodes) == 0 {
				// Return a non-nil node if we parsed "()".
				return []Node{Parameter{Value: ""}}, nil
			}
			return nodes, nil
		case s.match("and"), s.match("or"):
			// Caller advances.
			return nodes, nil
		default:
			value, err := s.scanParameter()
			if err != nil {
				return nil, err
			}
			if value != "" {
				nodes = append(nodes, Parameter{Value: value})
			}
		}
	}
	return nodes, nil
}

func reduce(left, right []Node, kind string) ([]Node, bool) {
	switch left[0].(type) {
	case Parameter:
		if left[0].(Parameter).Value == "" {
			// Remove empty string parameters.
			return right, true
		}
	}

	switch right[0].(type) {
	case Op:
		if kind == right[0].(Op).Kind {
			// Reduce right node.
			left = append(left, right[0].(Op).Children...)
			if len(right) > 1 {
				left = append(left, right[1:]...)
			}
			return left, true
		}
	case Parameter:
		if right[0].(Parameter).Value == "" {
			// Remove empty string parameters.
			if len(right) > 1 {
				return append(left, right[1:]...), true
			}
			return left, true
		}
		switch left[0].(type) {
		case Op:
			if kind == left[0].(Op).Kind {
				// Reduce left node.
				return append(left[0].(Op).Children, right...), true
			}
		}
	}
	if len(right) > 1 {
		// Reduce right list.
		reduced, changed := reduce(append(left, right[0]), right[1:], kind)
		if changed {
			return reduced, true
		}
	}
	return append(left, right...), false
}

func newOp(nodes []Node, kind string) []Node {
	if len(nodes) == 0 {
		return nil
	} else if len(nodes) == 1 {
		return nodes
	}

	reduced, changed := reduce([]Node{nodes[0]}, nodes[1:], kind)
	if changed {
		return newOp(reduced, kind)
	}

	return []Node{Op{Kind: kind, Children: reduced}}
}

func newAnd(nodes []Node) []Node {
	return newOp(nodes, "and")
}

func newOr(nodes []Node) []Node {
	return newOp(nodes, "or")
}

func (s *state) continueParsing(left []Node, operator string) ([]Node, error) {
	if left == nil {
		return nil, fmt.Errorf("expected operand at %d", s.pos)
	}

	var parse func() ([]Node, error)
	var newOp func(nodes []Node) []Node
	if operator == "and" {
		parse = s.parseAnd
		newOp = newAnd
	} else {
		parse = s.parseOr
		newOp = newOr
	}

	if s.done() || !s.expect(operator) {
		return newOp(left), nil
	}

	right, err := parse()
	if err != nil {
		return nil, err
	}
	return newOp(append(left, right...)), nil
}

func (s *state) parseAnd() ([]Node, error) {
	left, err := s.parseParameterList()
	if err != nil {
		return nil, err
	}
	return s.continueParsing(left, "and")
}

func (s *state) parseOr() ([]Node, error) {
	left, err := s.parseAnd()
	if err != nil {
		return nil, err
	}
	return s.continueParsing(left, "or")
}

func Parse(in string) ([]Node, error) {
	if in == "" {
		return nil, nil
	}
	state := &state{buf: []byte(in)}
	nodes, err := state.parseOr()
	if err != nil {
		return nil, err
	}
	if state.balanced != 0 {
		return nil, errors.New("unbalanced expression")
	}
	return nodes, nil
}
