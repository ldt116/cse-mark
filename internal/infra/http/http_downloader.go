package http

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/downloader"
)

type SimpleDownloader struct {
	Client *http.Client
}

// *SimpleDownloader must keep satisfying the authorized feed port that
// markimport consumes; guard the wider interface at compile time.
var _ downloader.AuthorizedRepository = (*SimpleDownloader)(nil)

func NewSimpleDownloader(config *configs.Config) downloader.Repository {
	return &SimpleDownloader{
		Client: &http.Client{
			Timeout: config.DownloaderTimeout,
		},
	}
}

func (d *SimpleDownloader) DownloadCSV(url string) ([][]string, error) {
	return d.download(url, "")
}

func (d *SimpleDownloader) DownloadCSVAuthorized(url string, token string) ([][]string, error) {
	return d.download(url, token)
}

// download performs the HTTP GET (with bearer token when token != "") and
// parses the CSV body. Non-2xx responses are surfaced as *downloader.FeedError
// so callers can classify config/permanent/transient failures.
func (d *SimpleDownloader) download(rawURL, token string) ([][]string, error) {
	// The CSV URL may carry a secret token in the path/query (e.g. a roster
	// link); log only its host, never the full URL.
	log.Info().Str("host", hostOf(rawURL)).Msg("Downloading CSV")

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		// *url.Error embeds the full URL (path/query secrets included) —
		// redact before logging or returning, same as transport errors below.
		log.Error().Err(redactURLErr(err)).Msg("Error building request")
		return nil, redactURLErr(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := d.Client.Do(req)
	if err != nil {
		// net/http wraps transport errors as *url.Error, whose Error() embeds
		// the full URL (token included). Unwrap to the underlying cause so the
		// secret URL is never written to logs.
		log.Error().Err(redactURLErr(err)).Msg("Error downloading link")
		return nil, redactURLErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		code := ""
		var envelope struct {
			Error string `json:"error"`
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if readErr == nil && json.Unmarshal(body, &envelope) == nil {
			code = envelope.Error
		}
		log.Warn().Int("status", resp.StatusCode).Str("code", code).Str("host", hostOf(rawURL)).Msg("Feed returned error status")
		return nil, &downloader.FeedError{Status: resp.StatusCode, Code: code}
	}

	// Parse the CSV data and extract URLs
	reader := csv.NewReader(resp.Body)

	records, err := reader.ReadAll()
	if err != nil {
		log.Error().Err(err).Msg("Error parsing csv")
		return nil, err
	}

	return records, nil
}

// hostOf returns the URL's host for logging, or "<invalid>" on a parse failure,
// so a malformed or secret-laden URL is never logged verbatim.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "<invalid>"
	}
	return u.Host
}

// redactURLErr strips the URL from a *url.Error, returning its underlying cause
// so the secret URL is never logged. Non-url errors are returned unchanged.
func redactURLErr(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Err != nil {
			return urlErr.Err
		}
		return errors.New(urlErr.Op + ": <redacted url>")
	}
	return err
}
