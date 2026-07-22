package courseadmin

import (
	"net/url"
)

// isValidURL accepts http(s) URLs with a host. It mirrors the loose validation
// the Telegram delivery layer does (url.ParseRequestURI) but explicitly requires
// a scheme so a bare "example.com/x.csv" is rejected.
func isValidURL(s string) bool {
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
