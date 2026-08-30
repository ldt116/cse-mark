package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/downloader"
)

// newDownloader builds a SimpleDownloader against a test config.
func newDownloader(t *testing.T) downloader.AuthorizedRepository {
	t.Helper()
	return NewSimpleDownloader(&configs.Config{DownloaderTimeout: 5 * time.Second}).(downloader.AuthorizedRepository)
}

func TestDownloadCSVAuthorized_SetsBearerHeader(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/csv")
		w.Write([]byte("id\nx\n"))
	}))
	defer srv.Close()

	dl := newDownloader(t)
	if _, err := dl.DownloadCSVAuthorized(srv.URL+"/feed", "secret-token"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer secret-token")
	}
	if gotPath != "/feed" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestDownloadCSVAuthorized_NoTokenSendsNoHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte("id\nx\n"))
	}))
	defer srv.Close()

	dl := newDownloader(t)
	if _, err := dl.DownloadCSVAuthorized(srv.URL, ""); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty (backward compat public CSV)", gotAuth)
	}
}

func TestDownloadCSVAuthorized_ErrorEnvelope(t *testing.T) {
	cases := []struct {
		status  int
		code    string
		rawBody string // "" => send proper JSON envelope
	}{
		{http.StatusUnauthorized, "service_token_invalid", ""},
		{http.StatusForbidden, "owner_token_invalid", ""},
		{http.StatusForbidden, "sheet_access_denied", ""},
		{http.StatusNotFound, "sheet_not_found", ""},
		{http.StatusGone, "grant_revoked", ""},
		{http.StatusTooManyRequests, "", ""}, // 429: proxy không trả, generic transient
		{http.StatusInternalServerError, "", ""},
		{http.StatusBadGateway, "", "<html>oops</html>"}, // non-JSON body → code rỗng
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.rawBody != "" {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.rawBody))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(tc.status)
			json.NewEncoder(w).Encode(map[string]string{"error": tc.code})
		}))
		defer srv.Close()
		dl := newDownloader(t)
		_, err := dl.DownloadCSVAuthorized(srv.URL, "tok")
		if err == nil {
			t.Fatalf("status %d: want error, got nil", tc.status)
		}
		var fe *downloader.FeedError
		if !errors.As(err, &fe) {
			t.Fatalf("status %d: err %v is not *FeedError", tc.status, err)
		}
		if fe.Status != tc.status || fe.Code != tc.code {
			t.Fatalf("status %d: FeedError = %+v, want {Status:%d Code:%q}", tc.status, fe, tc.status, tc.code)
		}
	}
}

func TestDownloadCSV_MalformedURLRedacted(t *testing.T) {
	// http.NewRequest fails on control chars in the URL; the returned
	// *url.Error embeds the full URL (path/query secrets included). The
	// downloader must strip it before returning or logging.
	dl := newDownloader(t).(downloader.Repository)
	_, err := dl.DownloadCSV("http://example.com/feed?token=SUPERSECRET\n")
	if err == nil {
		t.Fatal("want error for malformed URL, got nil")
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Fatalf("err leaks URL payload: %v", err)
	}
}

func TestDownloadCSV_Non2xxIsError(t *testing.T) {
	// Regression: trước đây non-2xx bị nạp thẳng vào csv.Reader → "invalid csv structure".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("nope"))
	}))
	defer srv.Close()

	dl := newDownloader(t).(downloader.Repository)
	_, err := dl.DownloadCSV(srv.URL)
	var fe *downloader.FeedError
	if !errors.As(err, &fe) || fe.Status != http.StatusNotFound {
		t.Fatalf("err = %v, want FeedError 404", err)
	}
}
