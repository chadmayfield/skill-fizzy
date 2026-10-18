package fizzy

import (
	"errors"
	"fmt"
)

// Refusal is an error fz raises on purpose before it sends a change: an
// ambiguous match, or a change it cannot make safely. The CLI exits 2 on a
// Refusal, as it does for a usage error.
type Refusal struct{ msg string }

func (e Refusal) Error() string { return e.msg }

// refusef returns a Refusal with a formatted message.
func refusef(format string, a ...any) error { return Refusal{fmt.Sprintf(format, a...)} }

// Refuse returns a Refusal with msg, for a check made outside this package.
func Refuse(msg string) error { return Refusal{msg} }

// IsRefusal reports whether err is or wraps a Refusal.
func IsRefusal(err error) bool {
	var r Refusal
	return errors.As(err, &r)
}
