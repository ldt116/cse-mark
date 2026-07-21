package http

import (
	"context"
	"encoding/csv"
	"errors"
	"net/http"
	"net/url"

	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/downloader"
)

type SimpleDownloader struct {
	Client *http.Client
	ctx    context.Context
}

func NewSimpleDownloader(config *configs.Config) downloader.Repository {
	return &SimpleDownloader{
		Client: &http.Client{
			Timeout: config.DownloaderTimeout,
		},
	}
}

func (d *SimpleDownloader) DownloadCSV(url string) ([][]string, error) {
	// The CSV URL may carry a secret token in the path/query (e.g. a roster
	// link); log only its host, never the full URL.
	log.Info().Str("host", hostOf(url)).Msg("Downloading CSV")

	// Make an HTTP GET request to the specified URL
	resp, err := http.Get(url)
	if err != nil {
		// net/http wraps transport errors as *url.Error, whose Error() embeds
		// the full URL (token included). Unwrap to the underlying cause so the
		// secret URL is never written to logs.
		log.Error().Err(redactURLErr(err)).Msg("Error downloading link")
		return nil, redactURLErr(err)
	}
	defer resp.Body.Close()

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
