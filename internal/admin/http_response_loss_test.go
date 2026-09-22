package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/auth"
)

type responseLossMode uint8

const (
	responseLossDisabled responseLossMode = iota
	responseLossComplete
	responseLossBody
)

// responseLossTransport deixa o handler real concluir a mutação e falha
// somente a entrega observada pelo cliente. Não é um proxy de produção.
type responseLossTransport struct {
	base     http.RoundTripper
	target   string
	mode     responseLossMode
	attempts atomic.Int32
}

func (t *responseLossTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil || response == nil || request.Method != http.MethodPost || request.URL.Path != t.target || t.mode == responseLossDisabled || t.attempts.Add(1) != 1 {
		return response, err
	}
	if t.mode == responseLossComplete {
		_ = response.Body.Close()
		return nil, errors.New("simulated complete response loss")
	}
	payload, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if len(payload) > 0 {
		payload = payload[:len(payload)-1]
	}
	response.Body = &truncatedResponseBody{payload: payload}
	response.ContentLength = -1
	response.Header.Del("Content-Length")
	return response, nil
}

type truncatedResponseBody struct {
	payload []byte
	offset  int
}

func (b *truncatedResponseBody) Read(dst []byte) (int, error) {
	if b.offset >= len(b.payload) {
		return 0, io.ErrUnexpectedEOF
	}
	count := copy(dst, b.payload[b.offset:])
	b.offset += count
	return count, nil
}

func (b *truncatedResponseBody) Close() error { return nil }

type responseLossClient struct {
	client *http.Client
	jar    *cookiejar.Jar
	loss   *responseLossTransport
}

func newResponseLossClient(target string, mode responseLossMode) *responseLossClient {
	jar, _ := cookiejar.New(nil)
	loss := &responseLossTransport{base: http.DefaultTransport, target: target, mode: mode}
	return &responseLossClient{
		client: &http.Client{Jar: jar, Transport: loss, Timeout: 5 * time.Second},
		jar:    jar,
		loss:   loss,
	}
}

func (c *responseLossClient) do(t *testing.T, method, path, payload, csrf string) (*http.Response, []byte, error) {
	t.Helper()
	var requestBody io.Reader
	if payload != "" {
		requestBody = strings.NewReader(payload)
	}
	request, err := http.NewRequest(method, "http://"+AdminAddress+path, requestBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "localhost:7677"
	if method == http.MethodPost {
		request.Header.Set("Origin", AdminOrigin)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	responseBody, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	return response, responseBody, readErr
}

type responseLossRequests struct{}

func (responseLossRequests) ListRequestSnapshots() []auth.RequestSnapshot { return nil }
func (responseLossRequests) GetRequestSnapshot(string) (auth.RequestSnapshot, error) {
	return auth.RequestSnapshot{}, nil
}
func (responseLossRequests) DecideAndSnapshot(string, int, string) (auth.RequestSnapshot, error) {
	return auth.RequestSnapshot{}, nil
}

func startResponseLossServer(t *testing.T, gate *Gate) {
	t.Helper()
	listener, err := net.Listen("tcp4", AdminAddress)
	if err != nil {
		t.Skipf("BLOQUEADO: listener fixo %s indisponível: %v", AdminAddress, err)
	}
	server := NewServer(gate.HandlerWithRequests(responseLossRequests{}))
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("shutdown do listener administrativo: %v", err)
		}
		if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("servidor administrativo terminou com erro: %v", err)
		}
		gate.Close()
		listener, err := net.Listen("tcp4", AdminAddress)
		if err != nil {
			t.Errorf("listener administrativo residual: %v", err)
			return
		}
		_ = listener.Close()
	})
}

func sessionState(t *testing.T, body []byte) (state, csrf string) {
	t.Helper()
	var model struct {
		State string `json:"state"`
		CSRF  string `json:"csrf_token"`
	}
	if err := json.Unmarshal(body, &model); err != nil {
		t.Fatalf("resposta de sessão inválida: %v", err)
	}
	return model.State, model.CSRF
}

func jarCookie(client *responseLossClient, name string) *http.Cookie {
	parsed, _ := url.Parse("http://localhost:7677/api/admin/v1/session")
	for _, cookie := range client.jar.Cookies(parsed) {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func cookieNames(cookies []*http.Cookie) []string {
	names := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		names = append(names, cookie.Name)
	}
	return names
}

func preparePairClient(t *testing.T, client *responseLossClient, code string) (string, string) {
	t.Helper()
	response, body, err := client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap HTTP: status=%v err=%v", responseStatus(response), err)
	}
	_, csrf := sessionState(t, body)
	payload, _ := json.Marshal(map[string]string{"pairing_code": code, "passphrase": testPassphrase})
	return csrf, string(payload)
}

func responseStatus(response *http.Response) any {
	if response == nil {
		return "transport-error"
	}
	return response.StatusCode
}

func runPairRecovery(t *testing.T, mode responseLossMode) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	startResponseLossServer(t, gate)
	client := newResponseLossClient("/api/admin/v1/pair", mode)
	csrf, payload := preparePairClient(t, client, code)
	response, _, requestErr := client.do(t, http.MethodPost, "/api/admin/v1/pair", payload, csrf)
	postResponse := response
	if mode == responseLossComplete {
		if requestErr == nil || response != nil {
			t.Fatalf("pareamento deveria perder a resposta inteira: status=%v err=%v", responseStatus(response), requestErr)
		}
	} else if requestErr == nil || response == nil {
		t.Fatalf("corpo do pareamento deveria ser inconclusivo: status=%v err=%v", responseStatus(response), requestErr)
	}
	_, paired, err := gate.Bootstrap("")
	if err != nil || !paired {
		t.Fatalf("servidor não consolidou o pareamento antes da perda: paired=%t err=%v", paired, err)
	}
	response, body, err := client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("reconciliação de pareamento: status=%v err=%v", responseStatus(response), err)
	}
	state, _ := sessionState(t, body)
	expected := "AUTHENTICATED"
	if mode == responseLossComplete {
		expected = "LOCKED"
		if jarCookie(client, adminCookie) != nil {
			t.Fatal("pareamento perdido por completo deixou cookie administrativo no cliente")
		}
	} else if jarCookie(client, adminCookie) == nil {
		t.Fatalf("resposta com Set-Cookie não entregou sessão administrativa ao cliente: response=%v jar=%v", cookieNames(postResponse.Cookies()), cookieNames(client.jar.Cookies(&url.URL{Scheme: "http", Host: "localhost:7677", Path: "/api/admin/v1/session"})))
	}
	if state != expected {
		t.Fatalf("estado reconciliado inesperado: got=%s want=%s", state, expected)
	}
	if client.loss.attempts.Load() != 1 {
		t.Fatalf("pareamento repetido após perda: posts=%d", client.loss.attempts.Load())
	}
	if mode == responseLossBody {
		response, _, err = client.do(t, http.MethodGet, "/api/admin/v1/requests", "", "")
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("cookie recebido não autenticou rota de pedidos: status=%v err=%v", responseStatus(response), err)
		}
	}
}

func runUnlockRecovery(t *testing.T, mode responseLossMode) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	startResponseLossServer(t, gate)
	client := newResponseLossClient("/api/admin/v1/unlock", responseLossDisabled)
	csrf, payload := preparePairClient(t, client, code)
	response, body, err := client.do(t, http.MethodPost, "/api/admin/v1/pair", payload, csrf)
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("pareamento de preparação: status=%v err=%v", responseStatus(response), err)
	}
	_, csrf = sessionState(t, body)
	response, body, err = client.do(t, http.MethodPost, "/api/admin/v1/lock", "{}", csrf)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("bloqueio de preparação: status=%v err=%v", responseStatus(response), err)
	}
	_, csrf = sessionState(t, body)
	client.loss.mode = mode
	response, _, requestErr := client.do(t, http.MethodPost, "/api/admin/v1/unlock", `{"passphrase":"`+testPassphrase+`"}`, csrf)
	postResponse := response
	if mode == responseLossComplete {
		if requestErr == nil || response != nil {
			t.Fatalf("desbloqueio deveria perder a resposta inteira: status=%v err=%v", responseStatus(response), requestErr)
		}
	} else if requestErr == nil || response == nil {
		t.Fatalf("corpo do desbloqueio deveria ser inconclusivo: status=%v err=%v", responseStatus(response), requestErr)
	}
	response, body, err = client.do(t, http.MethodGet, "/api/admin/v1/session", "", "")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("reconciliação de desbloqueio: status=%v err=%v", responseStatus(response), err)
	}
	state, _ := sessionState(t, body)
	expected := "AUTHENTICATED"
	if mode == responseLossComplete {
		expected = "LOCKED"
		if jarCookie(client, adminCookie) != nil {
			t.Fatal("desbloqueio perdido por completo deixou cookie administrativo no cliente")
		}
	} else if jarCookie(client, adminCookie) == nil {
		t.Fatalf("resposta de desbloqueio com Set-Cookie não chegou ao cliente: response=%v jar=%v", cookieNames(postResponse.Cookies()), cookieNames(client.jar.Cookies(&url.URL{Scheme: "http", Host: "localhost:7677", Path: "/api/admin/v1/session"})))
	}
	if state != expected {
		t.Fatalf("estado de desbloqueio reconciliado inesperado: got=%s want=%s", state, expected)
	}
	if client.loss.attempts.Load() != 1 {
		t.Fatalf("desbloqueio repetido após perda: posts=%d", client.loss.attempts.Load())
	}
	if mode == responseLossBody {
		response, _, err = client.do(t, http.MethodGet, "/api/admin/v1/requests", "", "")
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("cookie de desbloqueio recebido não autenticou pedidos: status=%v err=%v", responseStatus(response), err)
		}
	}
}

func TestAdminHTTPReconcilesPairAndUnlockAfterLostResponse(t *testing.T) {
	for _, scenario := range []struct {
		name string
		mode responseLossMode
	}{
		{name: "resposta inteira perdida", mode: responseLossComplete},
		{name: "cabecalho recebido corpo perdido", mode: responseLossBody},
	} {
		t.Run("pair/"+scenario.name, func(t *testing.T) { runPairRecovery(t, scenario.mode) })
		t.Run("unlock/"+scenario.name, func(t *testing.T) { runUnlockRecovery(t, scenario.mode) })
	}
}
