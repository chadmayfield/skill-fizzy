package fizzy

// CheckIDs refuses an id that cannot go into a URL path segment. An empty
// id drops the segment, and "." or ".." are dot segments that a proxy or
// server may remove, so the request would reach the parent resource.
// url.PathEscape leaves dots alone, so every call that puts an id from the
// user into a path checks it here first.
func CheckIDs(ids ...string) error {
	for _, id := range ids {
		switch id {
		case "", ".", "..":
			return refusef("%s is not an id; an id cannot be empty, \".\" or \"..\"", pyRepr(id))
		}
	}
	return nil
}
