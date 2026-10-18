package cli

import (
	"strconv"
	"strings"
)

// simple maps fz 0.1 command names to their noun form.
var simple = map[string][]string{
	"boards": {"board", "list"}, "board-create": {"board", "create"},
	"columns": {"column", "list"}, "cards": {"card", "list"},
	"show": {"card", "show"}, "create": {"card", "create"},
	"move": {"card", "move"}, "reopen": {"card", "reopen"},
	"golden": {"card", "golden"}, "delete": {"card", "delete"},
}

// commandIndex returns the index of the first argument after the global
// flags.
func commandIndex(args []string) int {
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--dry-run" || a == "--json" || strings.HasPrefix(a, "--config=") || isBoolFlagValue(a):
			i++
		case a == "--config":
			i += 2
		default:
			return i
		}
	}
	return i
}

// isBoolFlagValue reports whether a is --json or --dry-run with a value,
// such as --json=true, in a form the flag parser accepts.
func isBoolFlagValue(a string) bool {
	for _, f := range []string{"--json=", "--dry-run="} {
		if v, ok := strings.CutPrefix(a, f); ok {
			_, err := strconv.ParseBool(v)
			return err == nil
		}
	}
	return false
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// oldStep rewrites the arguments after "step" in "step N TEXT", "step N
// --check ID" and "step N --uncheck ID" (or --check=ID).
func oldStep(rest []string) []string {
	for _, f := range []struct{ flag, sub string }{{"--check", "check"}, {"--uncheck", "uncheck"}} {
		for j, a := range rest {
			if a == f.flag && j+1 < len(rest) {
				others := append(append([]string{}, rest[:j]...), rest[j+2:]...)
				return append([]string{"step", f.sub, others[0], rest[j+1]}, others[1:]...)
			}
			if strings.HasPrefix(a, f.flag+"=") {
				others := append(append([]string{}, rest[:j]...), rest[j+1:]...)
				return append([]string{"step", f.sub, others[0], strings.SplitN(a, "=", 2)[1]}, others[1:]...)
			}
		}
	}
	return append([]string{"step", "add"}, rest...)
}

// valueFlags are the flags that can sit after "tag" and take the next word
// as their value: the root's persistent --config. card tag's own flags take
// no value.
var valueFlags = map[string]bool{"--config": true}

// oldTag rewrites the arguments after "tag" in "tag N TAG" to "card tag N
// TAG". Flags before the card number, such as "tag --remove N TAG", move to
// the end with their values. Everything from the number on keeps its order.
// It reports false unless the first word that is not a flag or a flag value
// is a card number, so "tag list" stays as it is. Later words are tags, even
// "list".
func oldTag(rest []string) ([]string, bool) {
	var lead []string
	i := 0
	for i < len(rest) && strings.HasPrefix(rest[i], "-") {
		lead = append(lead, rest[i])
		if valueFlags[rest[i]] && i+1 < len(rest) {
			lead = append(lead, rest[i+1])
			i++
		}
		i++
	}
	if i >= len(rest) || !isNumber(rest[i]) {
		return nil, false
	}
	out := append([]string{"card", "tag"}, rest[i:]...)
	return append(out, lead...), true
}

// Rewrite turns fz 0.1 command lines into the noun form before cobra parses them.
func Rewrite(args []string) []string {
	i := commandIndex(args)
	if i >= len(args) {
		return args
	}
	head := append([]string{}, args[:i]...)
	cmd, rest := args[i], args[i+1:]
	if to, ok := simple[cmd]; ok {
		return append(append(head, to...), rest...)
	}
	if cmd == "tag" {
		if to, ok := oldTag(rest); ok {
			return append(head, to...)
		}
		return args
	}
	numbered := len(rest) > 0 && isNumber(rest[0])
	switch {
	case cmd == "step" && numbered:
		return append(head, oldStep(rest)...)
	case cmd == "comment" && numbered:
		return append(append(head, "comment", "add"), rest...)
	}
	return args
}
