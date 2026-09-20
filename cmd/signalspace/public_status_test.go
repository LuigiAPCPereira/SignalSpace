package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedPublicStatusIsRoutedWithoutAdministrativeAccess(t *testing.T) {
	handler, authorization, err := embeddedHandler(readTestResource, filepath.Join(t.TempDir(), "identity"))
	if err != nil {
		t.Fatal(err)
	}
	defer authorization.Close()
	for _, path := range []string{"/api/admin/v1/session", "/api/admin/v1/pair", "/api/admin/v1/requests", "/admin"} {
		response := readRequest(t, handler, http.MethodGet, path, "", "", "", &http.Cookie{Name: "signalspace_admin_session", Value: "not-an-admin-session"})
		if response.Code != http.StatusNotFound || len(response.Result().Cookies()) != 0 {
			t.Fatalf("administrative path exposed by public router: %s => %d", path, response.Code)
		}
	}
	registration := fmt.Sprintf(`{"client_name":"Status client","redirect_uris":[%q],"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`, readTestCallback)
	registered := readRequest(t, handler, http.MethodPost, "/register", registration, "application/json", "", nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register: %d", registered.Code)
	}
	var client struct {
		ID string `json:"client_id"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &client); err != nil || client.ID == "" {
		t.Fatal("client registration did not return ID")
	}
	hash := sha256.Sum256([]byte(readTestVerifier))
	query := url.Values{
		"client_id": {client.ID}, "redirect_uri": {readTestCallback}, "response_type": {"code"},
		"scope": {"signalspace:diagnostic"}, "resource": {readTestResource},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])},
		"code_challenge_method": {"S256"}, "state": {"random-state-identifier-for-status-test"},
	}
	consent := readRequest(t, handler, http.MethodGet, "/authorize?"+query.Encode(), "", "", "", nil)
	if consent.Code != http.StatusOK {
		t.Fatalf("authorize: %d", consent.Code)
	}
	id := regexp.MustCompile(`approve ([A-Za-z0-9_-]{22})`).FindStringSubmatch(consent.Body.String())
	if len(id) != 2 || len(consent.Result().Cookies()) != 1 {
		t.Fatal("missing OAuth request or session cookie")
	}
	status := readRequest(t, handler, http.MethodGet, "/authorize/status?request_id="+id[1], "", "", "", consent.Result().Cookies()[0])
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"PENDING"`) {
		t.Fatalf("public status not composed: %d %s", status.Code, status.Body.String())
	}
	if strings.Contains(status.Body.String(), client.ID) || strings.Contains(status.Body.String(), "csrf_token") {
		t.Fatal("public status leaked administrative or client metadata")
	}
}
