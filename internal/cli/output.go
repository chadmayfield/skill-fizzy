package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/chadmayfield/skill-fizzy/internal/client"
	"github.com/chadmayfield/skill-fizzy/internal/config"
	"github.com/chadmayfield/skill-fizzy/internal/fizzy"
	"github.com/spf13/cobra"
)

// cmdContext holds what a command needs. The config is loaded on first use,
// so commands that never call config or api run without a config file.
type cmdContext struct {
	cmd    *cobra.Command
	cfg    *config.Config
	fz     *fizzy.Fizzy
	dryRun bool
}

func newContext(cmd *cobra.Command) *cmdContext {
	return &cmdContext{cmd: cmd, dryRun: flagValue(cmd, "dry-run") == "true"}
}

// wrote returns text, the line a command prints after a write. In dry-run
// mode nothing was sent, so it returns "dry-run: nothing sent; would "
// followed by would instead.
func (c *cmdContext) wrote(text, would string) string {
	if c.dryRun {
		return "dry-run: nothing sent; would " + would
	}
	return text
}

// dry adds "dry_run": true to a command's JSON in dry-run mode. The
// commands added after the Python port pass their JSON through it. The
// commands the parity goldens pin do not.
func (c *cmdContext) dry(data any) any {
	if o, ok := data.(obj); ok && c.dryRun {
		return append(append(obj{}, o...), kv{"dry_run", true})
	}
	return data
}

// config loads the config file named by --config, or the default.
func (c *cmdContext) config() (*config.Config, error) {
	if c.cfg == nil {
		cfg, err := config.Load(flagValue(c.cmd, "config"))
		if err != nil {
			return nil, err
		}
		c.cfg = cfg
	}
	return c.cfg, nil
}

// inbox returns the inbox folder. It reads only the inbox setting, so a
// command that never reaches the server needs no URL or account.
func (c *cmdContext) inbox() (string, error) {
	if c.cfg != nil {
		return c.cfg.InboxPath(), nil
	}
	return config.LoadInbox(flagValue(c.cmd, "config"))
}

// api returns the Fizzy operations over a client for the loaded config.
// Dry-run lines go to stderr, as in the Python client.
func (c *cmdContext) api() (*fizzy.Fizzy, error) {
	if c.fz == nil {
		cfg, err := c.config()
		if err != nil {
			return nil, err
		}
		c.fz = &fizzy.Fizzy{C: client.New(cfg, c.dryRun, c.cmd.ErrOrStderr())}
	}
	return c.fz, nil
}

// flagValue reads a flag on cmd or a persistent flag of its parents, before
// or after cobra has merged them.
func flagValue(cmd *cobra.Command, name string) string {
	if f := cmd.Flag(name); f != nil {
		return f.Value.String()
	}
	return ""
}

func (c *cmdContext) out() io.Writer  { return c.cmd.OutOrStdout() }
func (c *cmdContext) errw() io.Writer { return c.cmd.ErrOrStderr() }

// action wraps a command body so it gets a fresh context.
func action(f func(c *cmdContext, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error { return f(newContext(cmd), args) }
}

// emit prints data as JSON with --json, otherwise text. data is marshaled
// with encoding/json and then printed the way Python's json.dumps(indent=2)
// prints: key order as given, non-ASCII escaped. Pass raw server bytes
// (json.RawMessage) to print what the server sent, or obj for a value built
// here.
func emit(cmd *cobra.Command, data any, text string) error {
	if flagValue(cmd, "json") != "true" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), text)
		return err
	}
	return printJSON(cmd.OutOrStdout(), data)
}

func printJSON(w io.Writer, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	out, err := fizzy.PyIndent(b)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", out)
	return err
}

// kv is one key and value of an obj.
type kv struct {
	key string
	val any
}

// obj is a JSON object that keeps its keys in the order given, the way a
// Python dict literal does.
type obj []kv

// MarshalJSON writes the keys in order.
func (o obj) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(p.key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(p.val)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// rawList joins the server bytes of each item into a JSON array.
func rawList[T interface{ Raw() json.RawMessage }](items []T) json.RawMessage {
	parts := make([][]byte, len(items))
	for i, it := range items {
		parts[i] = it.Raw()
	}
	return json.RawMessage("[" + string(bytes.Join(parts, []byte(","))) + "]")
}

// orNull returns nil for "", which prints as null, the way Python prints the
// None that a dry-run create returns.
func orNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// orNone formats s the way a Python f-string formats None: "None" for "".
func orNone(s string) string {
	if s == "" {
		return "None"
	}
	return s
}

// line cleans server text that must stay on one line, such as a title or
// a name: config.TerminalLine.
func line(s string) string { return config.TerminalLine(s) }

// body cleans server text that may run over several lines, such as a
// comment body. It keeps the newlines and indents every line after the
// first by two spaces, so no line from the server starts at column 0.
// Trailing newlines are dropped.
func body(s string) string {
	lines := strings.Split(strings.TrimRight(config.TerminalText(s), "\n"), "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = "  " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

// cardLine formats a card the way the Python _card_line does. Server text
// is kept to the line, so a title cannot fake another card's line.
func cardLine(c fizzy.Card) string {
	state := "maybe"
	if c.Column != nil && c.Column.Name != "" {
		state = line(c.Column.Name)
	}
	// Fizzy reports a card in Not now with no column and postponed true.
	if c.Postponed && c.Column == nil {
		state = "not now"
	}
	if c.Closed {
		state = "done"
	}
	gold := ""
	if c.Golden {
		gold = " ★"
	}
	tags := ""
	if len(c.Tags) > 0 {
		tags = " #" + line(strings.Join(c.Tags, " #"))
	}
	return fmt.Sprintf("#%-5d [%s] %s%s%s", c.Number, state, line(c.Title), gold, tags)
}

// pyBool formats a bool the way Python prints one.
func pyBool(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

// pyStrip strips whitespace the way Python's str.strip() does.
func pyStrip(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) })
}

// pyRepr formats a string like Python's repr. It is fizzy.PyRepr, so fz
// has one copy.
func pyRepr(s string) string { return fizzy.PyRepr(s) }

// exitCode ends a command with a code after it has printed its own message.
type exitCode int

func (e exitCode) Error() string { return "exit status " + strconv.Itoa(int(e)) }

// usagef returns a usage error, which exits 2 like an argparse error.
func usagef(format string, a ...any) error { return usageError{fmt.Errorf(format, a...)} }

// checkName refuses an empty or blank name with a usage error, before
// anything is sent. what names the flag or argument in the message.
func checkName(what, name string) error {
	if strings.TrimSpace(name) == "" {
		return usagef("%s cannot be empty", what)
	}
	return nil
}

// readDescriptionFile reads the Markdown file a --description-file flag
// names, with Windows and old Mac line ends turned into newlines. A file
// that cannot be read is a usage error.
func readDescriptionFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", usagef("argument --description-file: %v", err)
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n"), nil
}

// cardNumber parses a card number argument.
func cardNumber(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, usagef("argument number: invalid int value: %s", pyRepr(s))
	}
	return n, nil
}

// args wraps a cobra argument check so its error exits 2.
func args(check cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, a []string) error {
		if err := check(cmd, a); err != nil {
			return usageError{err}
		}
		return nil
	}
}

// choice checks a flag value against the allowed values, as argparse does.
func choice(flag, value string, allowed []string) error {
	for _, a := range allowed {
		if a == value {
			return nil
		}
	}
	quoted := make([]string, len(allowed))
	for i, a := range allowed {
		quoted[i] = pyRepr(a)
	}
	return usagef("argument --%s: invalid choice: %s (choose from %s)", flag, pyRepr(value), strings.Join(quoted, ", "))
}

// errorText is the message printed after "fz: ". Config errors drop the
// "config error: " prefix the config package adds, to match Python.
func errorText(err error) string {
	msg := err.Error()
	if errors.Is(err, config.ErrConfig) {
		msg = strings.TrimPrefix(msg, config.ErrConfig.Error()+": ")
	}
	return msg
}

// examples formats example lines for cobra's Example field.
func examples(lines ...string) string {
	return "  " + strings.Join(lines, "\n  ")
}

// noChange returns text, the line a command prints when the change it was
// asked for is already in place. In dry-run mode it says that nothing was
// sent instead, as wrote does for a change.
func noChange(c *cmdContext, text string) string {
	if c.dryRun {
		return "dry-run: nothing sent; no change needed"
	}
	return text
}

// refuse prints the closing line of a refusal and returns exit code 2.
func refuse(c *cmdContext) error {
	fmt.Fprintln(c.errw(), "refusing without --yes")
	return exitCode(2)
}

// listText joins one line per item, or the empty text when there are none.
func listText[T any](items []T, line func(T) string, empty string) string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = line(it)
	}
	if len(out) == 0 {
		return empty
	}
	return strings.Join(out, "\n")
}
