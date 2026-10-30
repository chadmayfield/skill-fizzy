package client

import "io"

// Out returns the writer the client prints dry-run lines to, which the CLI
// sets to stderr. Callers use it for warnings that belong with those lines.
// It never returns nil.
func (c *Client) Out() io.Writer {
	if c.out == nil {
		return io.Discard
	}
	return c.out
}
