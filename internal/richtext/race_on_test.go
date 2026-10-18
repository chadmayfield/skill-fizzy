//go:build race

package richtext

import "time"

// timeLimit is how long the linear-time tests allow for 1 MB. The race
// detector makes each memory access many times slower, so a race build
// gets 10 s. The quadratic code these tests catch took minutes.
const timeLimit = 10 * time.Second
