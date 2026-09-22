package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

type expiryHTTPSession struct {
	State         string    `json:"state"`
	Authenticated bool      `json:"authenticated"`
	CSRF          string    `json:"csrf_token"`
	IdleUntil     time.Time `json:"idle_expires_at"`
	AbsoluteAt    time.Time `json:"absolute_expires_at"`
}

type expiryHTTPClient struct {
	base   string
	client *http.Client
}

func newExpiryHTTPClient(base string) *expiryHTTPClient {
	jar, _ := cookiejar.New(nil)
	return &expiryHTTPClient{
		base: base,
		client: &http.Client{
			Jar:       jar,
			Transport: &http.Transport{Proxy: nil},
			Timeout:   10 * time.Second,
		},
	}
}

func (c *expiryHTTPClient) do(t *testing.T, method, path, payload, csrf string) (int, []byte, error) {
	t.Helper()
	var body io.Reader
	if payload != "" {
		body = strings.NewReader(payload)
	}
	request, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return 0, nil, err
	}
	request.Host = "localhost:7677"
	if method == http.MethodPost {
		request.Header.Set("Origin", AdminOrigin)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	return response.StatusCode, responseBody, err
}

func startExpiryHTTPServer(t *testing.T, gate *Gate, requests OAuthRequests) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listener loopback efêmero: %v", err)
	}
	server := NewServer(gate.HandlerWithRequests(requests))
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("shutdown do listener efêmero: %v", err)
		}
		if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("servidor efêmero terminou com erro: %v", err)
		}
		gate.Close()
	})
	return "http://" + listener.Addr().String()
}

func decodeExpirySession(t *testing.T, body []byte) expiryHTTPSession {
	t.Helper()
	var session expiryHTTPSession
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatalf("resposta de sessão inválida: %v", err)
	}
	return session
}

func expiryErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("resposta de erro inválida: %v", err)
	}
	return response.Error.Code
}

func pairExpiryHTTPClient(t *testing.T, client *expiryHTTPClient, code string) expiryHTTPSession {
	t.Helper()
	status, body, err := client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
	if err != nil || status != http.StatusOK {
		t.Fatalf("bootstrap HTTP: status=%d err=%v", status, err)
	}
	var bootstrap struct {
		State string `json:"state"`
		CSRF  string `json:"csrf_token"`
	}
	if err := json.Unmarshal(body, &bootstrap); err != nil {
		t.Fatalf("bootstrap inválido: %v", err)
	}
	if bootstrap.State != "UNPAIRED" || bootstrap.CSRF == "" {
		t.Fatalf("bootstrap inesperado: state=%q csrf_present=%t", bootstrap.State, bootstrap.CSRF != "")
	}
	payload, err := json.Marshal(map[string]string{"pairing_code": code, "passphrase": testPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	status, body, err = client.do(t, http.MethodPost, "/api/admin/v1/pair", string(payload), bootstrap.CSRF)
	if err != nil || status != http.StatusCreated {
		t.Fatalf("pareamento HTTP: status=%d err=%v", status, err)
	}
	session := decodeExpirySession(t, body)
	if session.State != "AUTHENTICATED" || !session.Authenticated || session.CSRF == "" {
		t.Fatal("pareamento não produziu sessão autenticada")
	}
	return session
}

func assertExpiryUnauthorized(t *testing.T, status int, body []byte) {
	t.Helper()
	if status != http.StatusUnauthorized || expiryErrorCode(t, body) != "AUTH_REQUIRED" {
		t.Fatalf("resposta não autenticada inesperada: status=%d code=%q", status, expiryErrorCode(t, body))
	}
}

func assertNoExpiryRequestData(t *testing.T, body []byte, requestID string) {
	t.Helper()
	if bytes.Contains(body, []byte(requestID)) || bytes.Contains(body, []byte(`"requests"`)) {
		t.Fatal("resposta de sessão expirada expôs dados da fila")
	}
}

func assertExpiryRequestPending(t *testing.T, fixture *requestFixture) {
	t.Helper()
	item, err := fixture.GetRequestSnapshot(fixture.item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "PENDING" || item.Version != 1 {
		t.Fatalf("pedido foi alterado após expiração: status=%q version=%d", item.Status, item.Version)
	}
	fixture.mu.Lock()
	decisions := fixture.decisions
	fixture.mu.Unlock()
	if decisions != 0 {
		t.Fatalf("decisões aplicadas após expiração: %d", decisions)
	}
}

func TestAdminHTTPSessionExpiryAndAbsoluteRefresh(t *testing.T) {
	t.Run("idle expiration does not reactivate", func(t *testing.T) {
		gate, code, err := NewGate()
		if err != nil {
			t.Fatal(err)
		}
		clock := time.Now().UTC().Truncate(time.Second)
		gate.now = func() time.Time { return clock }
		fixture := newRequestFixture()
		client := newExpiryHTTPClient(startExpiryHTTPServer(t, gate, fixture))
		session := pairExpiryHTTPClient(t, client, code)
		requestID := fixture.item.ID

		clock = session.IdleUntil.Add(-time.Second)
		status, body, err := client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("GET sessão antes do idle: status=%d err=%v", status, err)
		}
		observed := decodeExpirySession(t, body)
		if !observed.IdleUntil.Equal(session.IdleUntil) || !observed.AbsoluteAt.Equal(session.AbsoluteAt) {
			t.Fatal("GET sessão renovou ou alterou os prazos")
		}
		status, body, err = client.do(t, http.MethodGet, requestsPath, "", "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("GET fila antes do idle: status=%d err=%v", status, err)
		}
		var list struct {
			Requests []auth.RequestSnapshot `json:"requests"`
		}
		if err := json.Unmarshal(body, &list); err != nil || len(list.Requests) != 1 || list.Requests[0].ID != requestID {
			t.Fatalf("fila autenticada inesperada: count=%d err=%v", len(list.Requests), err)
		}
		status, body, err = client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("GET sessão após fila: status=%d err=%v", status, err)
		}
		observed = decodeExpirySession(t, body)
		if !observed.IdleUntil.Equal(session.IdleUntil) {
			t.Fatal("GET fila renovou o idle implicitamente")
		}

		clock = session.IdleUntil
		status, body, err = client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
		if err != nil {
			t.Fatal(err)
		}
		assertExpiryUnauthorized(t, status, body)
		assertNoExpiryRequestData(t, body, requestID)

		status, body, err = client.do(t, http.MethodGet, requestsPath, "", "")
		if err != nil {
			t.Fatal(err)
		}
		assertExpiryUnauthorized(t, status, body)
		assertNoExpiryRequestData(t, body, requestID)

		decisionPayload := `{"decision":"approve","expected_version":1}`
		status, body, err = client.do(t, http.MethodPost, decisionPath(requestID), decisionPayload, session.CSRF)
		if err != nil {
			t.Fatal(err)
		}
		assertExpiryUnauthorized(t, status, body)
		assertNoExpiryRequestData(t, body, requestID)

		status, body, err = client.do(t, http.MethodPost, "/api/admin/v1/session/refresh", `{}`, session.CSRF)
		if err != nil {
			t.Fatal(err)
		}
		assertExpiryUnauthorized(t, status, body)
		assertNoExpiryRequestData(t, body, requestID)
		assertExpiryRequestPending(t, fixture)

		status, body, err = client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("bootstrap após expiração: status=%d err=%v", status, err)
		}
		locked := decodeExpirySession(t, body)
		if locked.State != "LOCKED" || locked.Authenticated {
			t.Fatalf("expiração reativou a sessão: state=%q authenticated=%t", locked.State, locked.Authenticated)
		}
	})

	t.Run("refresh cannot cross absolute deadline", func(t *testing.T) {
		gate, code, err := NewGate()
		if err != nil {
			t.Fatal(err)
		}
		clock := time.Now().UTC().Truncate(time.Second)
		gate.now = func() time.Time { return clock }
		fixture := newRequestFixture()
		client := newExpiryHTTPClient(startExpiryHTTPServer(t, gate, fixture))
		session := pairExpiryHTTPClient(t, client, code)
		requestID := fixture.item.ID
		absolute := session.AbsoluteAt

		clock = clock.Add(time.Minute)
		status, body, err := client.do(t, http.MethodPost, "/api/admin/v1/session/refresh", `{}`, "invalid-csrf")
		if err != nil || status != http.StatusForbidden || expiryErrorCode(t, body) != "ACCESS_DENIED" {
			t.Fatalf("refresh com CSRF inválido: status=%d err=%v", status, err)
		}

		for _, offset := range []time.Duration{10, 20, 30, 31, 45} {
			clock = session.AbsoluteAt.Add(time.Duration(offset-60) * time.Minute)
			status, body, err = client.do(t, http.MethodPost, "/api/admin/v1/session/refresh", `{}`, session.CSRF)
			if err != nil || status != http.StatusOK {
				t.Fatalf("refresh no minuto %d: status=%d err=%v", offset, status, err)
			}
			refreshed := decodeExpirySession(t, body)
			if !refreshed.AbsoluteAt.Equal(absolute) {
				t.Fatalf("refresh alterou prazo absoluto no minuto %d", offset)
			}
			session = refreshed
		}
		if !session.IdleUntil.Equal(absolute) {
			t.Fatal("refresh ultrapassou ou não alcançou o limite absoluto")
		}

		status, body, err = client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("GET sessão antes do absoluto: status=%d err=%v", status, err)
		}
		observed := decodeExpirySession(t, body)
		status, body, err = client.do(t, http.MethodGet, requestsPath, "", "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("GET fila antes do absoluto: status=%d err=%v", status, err)
		}
		status, body, err = client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("GET sessão após fila: status=%d err=%v", status, err)
		}
		refreshed := decodeExpirySession(t, body)
		if !observed.IdleUntil.Equal(refreshed.IdleUntil) || !refreshed.IdleUntil.Equal(absolute) || !refreshed.AbsoluteAt.Equal(absolute) {
			t.Fatal("GET sessão/fila renovou o idle ou alterou o absoluto")
		}

		clock = absolute
		status, body, err = client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
		if err != nil {
			t.Fatal(err)
		}
		assertExpiryUnauthorized(t, status, body)
		assertNoExpiryRequestData(t, body, requestID)

		status, body, err = client.do(t, http.MethodGet, requestsPath, "", "")
		if err != nil {
			t.Fatal(err)
		}
		assertExpiryUnauthorized(t, status, body)
		assertNoExpiryRequestData(t, body, requestID)
		status, body, err = client.do(t, http.MethodPost, "/api/admin/v1/session/refresh", `{}`, session.CSRF)
		if err != nil {
			t.Fatal(err)
		}
		assertExpiryUnauthorized(t, status, body)
		assertNoExpiryRequestData(t, body, requestID)
		status, body, err = client.do(t, http.MethodPost, decisionPath(requestID), `{"decision":"approve","expected_version":1}`, session.CSRF)
		if err != nil {
			t.Fatal(err)
		}
		assertExpiryUnauthorized(t, status, body)
		assertNoExpiryRequestData(t, body, requestID)
		assertExpiryRequestPending(t, fixture)

		status, body, err = client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
		if err != nil || status != http.StatusOK {
			t.Fatalf("bootstrap após absoluto: status=%d err=%v", status, err)
		}
		locked := decodeExpirySession(t, body)
		if locked.State != "LOCKED" || locked.Authenticated {
			t.Fatalf("absoluto reativou a sessão: state=%q authenticated=%t", locked.State, locked.Authenticated)
		}
	})
}
