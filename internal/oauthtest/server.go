// Package oauthtest fakes the token endpoints of the Microsoft identity
// platform (device code + refresh token grants) for tests.
package oauthtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// Values the fake hands out.
const (
	ClientID        = "test-client-id"
	UserCode        = "ABCD-EFGH"
	VerificationURI = "https://microsoft.com/devicelogin"
	deviceCode      = "device-code-123"
)

// Request is one request the fake received.
type Request struct {
	Tenant string
	Path   string
	Form   url.Values
}

// Server is a running fake. Access and refresh tokens are numbered:
// the n-th issued pair is "access-n" / "refresh-n".
type Server struct {
	URL    string
	Client *http.Client

	mu sync.Mutex
	// ExpiresIn is the lifetime in seconds of issued access tokens.
	ExpiresIn int
	// OmitRefreshToken leaves the refresh token out of token responses.
	OmitRefreshToken bool
	// FailDeviceCode makes the device code request fail.
	FailDeviceCode bool
	issued         int
	refreshToken   string
	requests       []Request
}

// Start runs the fake until the test ends.
func Start(t testing.TB) *Server {
	t.Helper()
	s := &Server{ExpiresIn: 3600}
	srv := httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(srv.Close)
	s.URL, s.Client = srv.URL, srv.Client()
	return s
}

// Requests returns the requests received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// Set changes the fake's behaviour under its lock.
func (s *Server) Set(f func(*Server)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, "invalid_request")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tenant, path, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	s.requests = append(s.requests, Request{Tenant: tenant, Path: path, Form: r.PostForm})
	if r.PostForm.Get("client_id") != ClientID {
		writeError(w, "invalid_client")
		return
	}

	switch path {
	case "oauth2/v2.0/devicecode":
		if s.FailDeviceCode {
			writeError(w, "unauthorized_client")
			return
		}
		writeJSON(w, map[string]any{
			"device_code":      deviceCode,
			"user_code":        UserCode,
			"verification_uri": VerificationURI,
			"expires_in":       900,
			"interval":         1,
		})
	case "oauth2/v2.0/token":
		switch r.PostForm.Get("grant_type") {
		case "urn:ietf:params:oauth:grant-type:device_code":
			if r.PostForm.Get("device_code") != deviceCode {
				writeError(w, "invalid_grant")
				return
			}
		case "refresh_token":
			if s.refreshToken == "" || r.PostForm.Get("refresh_token") != s.refreshToken {
				writeError(w, "invalid_grant")
				return
			}
		default:
			writeError(w, "unsupported_grant_type")
			return
		}
		s.issue(w)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) issue(w http.ResponseWriter) {
	s.issued++
	resp := map[string]any{
		"token_type":   "Bearer",
		"access_token": fmt.Sprintf("access-%d", s.issued),
		"expires_in":   s.ExpiresIn,
	}
	s.refreshToken = ""
	if !s.OmitRefreshToken {
		s.refreshToken = fmt.Sprintf("refresh-%d", s.issued)
		resp["refresh_token"] = s.refreshToken
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": code})
}
