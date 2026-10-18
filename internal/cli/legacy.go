package cli

import "strings"

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
		case a == "--dry-run" || a == "--json" || strings.HasPrefix(a, "--config="):
			i++
		case a == "--config":
			i += 2
		default:
			return i
		}
	}
	return i
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

// tagSubcommands are the words after "tag" that name a tag command, so
// "tag list" is never read as an old card tag line.
var tagSubcommands = map[string]bool{"list": true}

// oldTag rewrites the arguments after "tag" in "tag N TAG", with flags
// anywhere, such as "tag --remove N TAG", to "card tag N TAG" with the flags
// last. It reports false unless the first word that is not a flag is a card
// number and no word names a tag subcommand.
func oldTag(rest []string) ([]string, bool) {
	var words, flags []string
	for _, a := range rest {
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
		} else {
			words = append(words, a)
		}
	}
	if len(words) == 0 || !isNumber(words[0]) {
		return nil, false
	}
	for _, w := range words {
		if tagSubcommands[w] {
			return nil, false
		}
	}
	return append(append([]string{"card", "tag"}, words...), flags...), true
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
