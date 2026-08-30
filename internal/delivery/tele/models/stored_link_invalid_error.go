package models

import "errors"

// ErrStoredLinkInvalid is returned when a course's stored CSV link fails URI
// validation. It replaces the raw *url.Error from url.ParseRequestURI, whose
// message embeds the full link (a proxy token may sit in its path/query) — the
// error text must stay link-free because the middleware echoes it into the
// chat.
var ErrStoredLinkInvalid = errors.New("stored course link is invalid")
