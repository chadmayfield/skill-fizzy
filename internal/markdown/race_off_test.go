//go:build !race

package markdown

import "time"

// timeLimit is how long the linear-time tests allow for 1 MB.
const timeLimit = time.Second
