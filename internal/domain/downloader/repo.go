package downloader

import "strconv"

type Repository interface {
	DownloadCSV(url string) ([][]string, error)
}

// AuthorizedRepository downloads CSV feeds that may require a bearer token
// (mark feeds served by the hcmut-util gv proxy).
type AuthorizedRepository interface {
	// DownloadCSVAuthorized fetches the CSV at url. When token is empty no
	// Authorization header is sent (public feeds keep working unchanged).
	// Non-2xx responses return *FeedError.
	DownloadCSVAuthorized(url string, token string) ([][]string, error)
}

// FeedError describes a non-2xx feed response. Code carries the machine
// readable error code from the JSON envelope {"error":"<code>"} when present.
type FeedError struct {
	Status int
	Code   string
}

func (e *FeedError) Error() string {
	if e.Code != "" {
		return "feed error: http " + strconv.Itoa(e.Status) + " code " + e.Code
	}
	return "feed error: http " + strconv.Itoa(e.Status)
}
