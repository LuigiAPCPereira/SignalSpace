package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	diagnosticScope      = "signalspace:diagnostic"
	workspaceReadScope   = "signalspace:workspace.read"
	workspaceWriteScope  = "signalspace:workspace.write"
	gitReviewScope       = "signalspace:git.review"
	testRunScope         = "signalspace:test.run"
	maxRegistrationBytes = 16 << 10
	maxFormBytes         = 8 << 10
	maxClients           = 128
	maxPending           = 32
	maxCodes             = 64
	pendingTTL           = 5 * time.Minute
	codeTTL              = time.Minute
	tokenTTL             = 15 * time.Minute
	ownerSubject         = "local-owner"
	quotaWindow          = time.Minute
	consentScriptPath    = "/authorize/consent.js"
)

var (
	pkceChallenge = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	pkceVerifier  = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
	requestID     = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
	consentPage   = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Autorizar SignalSpace</title>
</head>
<body style="font:16px system-ui;max-width:38rem;margin:10vh auto;padding:1rem;line-height:1.5">
<main id="consent" data-request-id="{{.ID}}">
<h1>Autorizar conexão</h1>
<p>Solicitação de <strong>{{.Client}}</strong>.</p>
<p>Registro OAuth: <code>{{.ClientID}}</code></p>
{{if .Read}}<p><strong>Permissão adicional:</strong> ler arquivos de texto da pasta autorizada separadamente no terminal. Esta permissão não dá acesso a outras pastas, edição ou shell.</p>{{end}}
{{if .Write}}<p><strong>Permissão adicional:</strong> modificar arquivos de texto dentro da pasta autorizada. Esta permissão não dá acesso a outras pastas, comandos ou Git.</p>{{end}}
{{if .Test}}<p><strong>Permissão adicional:</strong> executar o teste predefinido do projeto autorizado. Os testes podem executar código com os privilégios do usuário. O workspace não é sandbox.</p>{{end}}
{{if .Git}}<p><strong>Permissão adicional:</strong> inspecionar status e diff Git do workspace. O diff pode conter conteúdo sensível. Esta permissão não autoriza commit ou push.</p>{{end}}
{{if and (not .Read) (not .Write) (not .Test) (not .Git)}}<p>Permissão solicitada: somente diagnóstico de conexão, sem acesso a arquivos.</p>{{end}}
<p>Escopos solicitados: <code>{{.Scope}}</code></p>
<p>Destino do retorno: <code>{{.Redirect}}</code></p>
<p id="authorization-status" role="status" aria-live="polite">Confirme na janela do terminal em que o SignalSpace está em execução.</p>
<pre>approve {{.ID}}</pre>
<p>Depois que a aprovação local for confirmada, clique em Continuar. Para recusar, digite <code>deny {{.ID}}</code> no terminal.</p>
<form id="authorization-complete-form" method="post" action="/authorize/complete">
<input type="hidden" name="request" value="{{.ID}}">
<input type="hidden" name="csrf" value="{{.CSRF}}">
<button id="continue-button" type="submit" disabled>Continuar</button>
<noscript><p>JavaScript está desativado; a atualização automática não está disponível. O servidor continuará validando a aprovação local antes de concluir.</p><button type="submit">Continuar sem atualização automática</button></noscript>
</form>
<p>Esta solicitação expira em cinco minutos. Nenhum acesso é concedido antes da aprovação local.</p>
</main>
<script src="/authorize/consent.js" defer></script>
</body>
</html>`))
)

//go:embed consent.js
var consentScript []byte

type Config struct {
	ResourceURL string
	Issuer      string
	Scope       string
	// ReadScope é opcional e só pode ser oferecido junto de um verificador de
	// concessões locais. A configuração padrão permanece somente diagnóstico.
	ReadScope    string
	CanIssueRead func(clientID string) bool
	// WriteScope é uma extensão experimental: só pode ser configurado junto de
	// um verificador local explícito. A composição padrão nunca o preenche.
	WriteScope    string
	CanIssueWrite func(clientID string) bool
	// GitScope é uma extensão experimental: só pode ser oferecida junto de um
	// verificador local explícito. A configuração padrão nunca a preenche.
	GitScope    string
	CanIssueGit func(clientID string) bool
	// TestScope é uma extensão experimental: só pode ser oferecida junto de um
	// verificador local explícito. A configuração padrão nunca a preenche.
	TestScope    string
	CanIssueTest func(clientID string) bool
	StateDir     string
	OnRequest    func(RequestInfo)
	// OnRegistrationFailure recebe somente categorias fixas, nunca metadados do cliente.
	OnRegistrationFailure func(string)
}

type RequestInfo struct{ ID, Client, ClientID, Redirect, Scope string }

// ClientInfo contém apenas metadados já registrados, exibidos somente no terminal.
type ClientInfo struct {
	ID   string
	Name string
}
type client struct {
	Name      string
	Redirects []string
}
type pending struct {
	ClientID, Redirect, Challenge, State, Resource, Scope string
	SessionHash                                           [32]byte
	CSRF                                                  string
	Expires                                               time.Time
	CreatedAt                                             time.Time
	DecidedAt                                             time.Time
	Version                                               int
	Approved                                              bool
	Denied                                                bool
}
type grant struct {
	ClientID, Redirect, Challenge, Resource, Scope string
	Expires                                        time.Time
}

type requestQuota struct {
	Started time.Time
	Count   int
}

type Server struct {
	config   Config
	key      *rsa.PrivateKey
	keyID    string
	store    *identityStore
	mu       sync.Mutex
	clients  map[string]client
	issued   map[string]bool
	pending  map[string]pending
	terminal map[string]terminalRecord
	codes    map[string]grant
	quotas   map[string]requestQuota
}

func New(config Config) (*Server, error) {
	resource, err := url.Parse(config.ResourceURL)
	if err != nil || resource.Scheme != "https" || resource.Hostname() == "" || resource.User != nil || resource.Path != "/mcp" || resource.RawPath != "" || resource.RawQuery != "" || resource.ForceQuery || resource.Fragment != "" || resource.String() != config.ResourceURL {
		return nil, errors.New("embedded authorization requires canonical HTTPS resource /mcp")
	}
	if config.Issuer != "https://"+resource.Host || config.Scope == "" {
		return nil, errors.New("embedded issuer must equal resource HTTPS origin and scope must be set")
	}
	if (config.ReadScope != "" && (config.Scope != diagnosticScope || config.ReadScope != workspaceReadScope || config.CanIssueRead == nil || config.OnRequest == nil)) || (config.ReadScope == "" && config.CanIssueRead != nil) {
		return nil, errors.New("workspace read scope requires an explicit local grant validator")
	}
	if (config.WriteScope != "" && (config.Scope != diagnosticScope || config.WriteScope != workspaceWriteScope || config.CanIssueWrite == nil || config.OnRequest == nil)) || (config.WriteScope == "" && config.CanIssueWrite != nil) {
		return nil, errors.New("workspace write scope requires an explicit local grant validator")
	}
	if (config.GitScope != "" && (config.Scope != diagnosticScope || config.GitScope != gitReviewScope || config.CanIssueGit == nil || config.OnRequest == nil)) || (config.GitScope == "" && config.CanIssueGit != nil) {
		return nil, errors.New("Git review scope requires an explicit local grant validator")
	}
	if (config.TestScope != "" && (config.Scope != diagnosticScope || config.TestScope != testRunScope || config.CanIssueTest == nil || config.OnRequest == nil)) || (config.TestScope == "" && config.CanIssueTest != nil) {
		return nil, errors.New("test execution scope requires an explicit local grant validator")
	}
	store, key, kid, clients, err := openIdentity(config.StateDir, config)
	if err != nil {
		return nil, err
	}
	return &Server{config: config, key: key, keyID: kid, store: store, clients: clients, issued: make(map[string]bool), pending: make(map[string]pending), terminal: make(map[string]terminalRecord), codes: make(map[string]grant), quotas: make(map[string]requestQuota)}, nil
}

// Close libera a trava do estado; não preserva códigos e aprovações temporárias.
func (s *Server) Close() error { return s.store.Close() }

func (s *Server) PublicKey() *rsa.PublicKey { return &s.key.PublicKey }
func (s *Server) KeyID() string             { return s.keyID }
func (s *Server) OwnerSubject() string      { return ownerSubject }

// IssuedClients retorna somente registros para os quais esta instância gerou
// um token após consentimento e troca válida do código. Não atesta o software
// cliente e não implica consentimento de workspace ou escopo de leitura.
func (s *Server) IssuedClients() []ClientInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	clients := make([]ClientInfo, 0, len(s.issued))
	for id := range s.issued {
		if registered, ok := s.clients[id]; ok {
			clients = append(clients, ClientInfo{ID: id, Name: registered.Name})
		}
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].ID < clients[j].ID })
	return clients
}

func randomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func jsonReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func bad(w http.ResponseWriter, status int, kind string) {
	jsonReply(w, status, map[string]string{"error": kind})
}

// rejectRegistration publica apenas o motivo enumerado, sem repetir campos não confiáveis.
func (s *Server) rejectRegistration(w http.ResponseWriter, kind, reason string) {
	if s.config.OnRegistrationFailure != nil {
		s.config.OnRegistrationFailure(reason)
	}
	jsonReply(w, http.StatusBadRequest, map[string]string{"error": kind, "error_description": reason})
}

// supportedRegistrationGrants negocia refresh_token para fora: não emitir nem
// anunciar uma concessão que o servidor de tokens ainda não implementa.
func supportedRegistrationGrants(grants []string) bool {
	if len(grants) == 0 {
		return true
	}
	if len(grants) > 2 {
		return false
	}
	authorizationCode, refresh := false, false
	for _, grant := range grants {
		switch grant {
		case "authorization_code":
			if authorizationCode {
				return false
			}
			authorizationCode = true
		case "refresh_token":
			if refresh {
				return false
			}
			refresh = true
		default:
			return false
		}
	}
	return authorizationCode
}
func hasContentType(r *http.Request, expected string) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == expected
}
func allowedRedirect(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), "chatgpt.com") && u.Port() == "" && u.User == nil && u.Path != "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && u.String() == raw
}
func (s *Server) clean(now time.Time) {
	s.expireRequestsLocked(now)
	for id, c := range s.codes {
		if !now.Before(c.Expires) {
			delete(s.codes, id)
		}
	}
}

// Approve é um alias legado do terminal; não pode contornar a decisão única.
func (s *Server) Approve(id string, allow bool) error {
	return s.DecideTerminal(id, allow)
}

// Handler contém somente os endpoints públicos de autorização. Aprovação não é uma rota HTTP.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", s.metadata)
	mux.HandleFunc("/oauth/jwks", s.jwks)
	mux.HandleFunc("/register", s.register)
	mux.HandleFunc("/authorize", s.authorize)
	mux.HandleFunc(consentScriptPath, s.consentScript)
	mux.HandleFunc("/authorize/complete", s.complete)
	mux.HandleFunc("/token", s.token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != strings.TrimPrefix(s.config.Issuer, "https://") || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.config.Issuer) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// O IP do proxy pode ser compartilhado: cotas globais não confiam em Forwarded.
		if !s.allowPublicRequest(r.URL.Path, time.Now()) {
			w.Header().Set("Retry-After", "60")
			bad(w, http.StatusTooManyRequests, "slow_down")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) consentScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write(consentScript)
}

// allowPublicRequest usa um mapa fixo: nenhuma chave vem de IP ou entrada remota.
func (s *Server) allowPublicRequest(path string, now time.Time) bool {
	limit := 0
	switch path {
	case "/register":
		limit = 16
	case "/authorize":
		limit = 64
	case "/authorize/complete", "/token":
		limit = 128
	default:
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	quota := s.quotas[path]
	if quota.Started.IsZero() || now.Sub(quota.Started) >= quotaWindow {
		quota = requestQuota{Started: now}
	}
	if quota.Count >= limit {
		return false
	}
	quota.Count++
	s.quotas[path] = quota
	return true
}
func (s *Server) metadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	i := s.config.Issuer
	scopes := []string{s.config.Scope}
	if s.config.ReadScope != "" {
		scopes = append(scopes, s.config.ReadScope)
	}
	if s.config.WriteScope != "" {
		scopes = append(scopes, s.config.WriteScope)
	}
	if s.config.GitScope != "" {
		scopes = append(scopes, s.config.GitScope)
	}
	if s.config.TestScope != "" {
		scopes = append(scopes, s.config.TestScope)
	}
	jsonReply(w, 200, map[string]any{"issuer": i, "authorization_endpoint": i + "/authorize", "token_endpoint": i + "/token", "registration_endpoint": i + "/register", "jwks_uri": i + "/oauth/jwks", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": scopes, "authorization_response_iss_parameter_supported": true})
}

type capabilities struct {
	Read  bool
	Write bool
	Git   bool
	Test  bool
}

// requestedCapabilities aceita qualquer subconjunto explicitamente configurado
// na ordem canônica da metadata: diagnóstico, leitura, escrita, Git, teste.
// Escopos desconhecidos, duplicados, não configurados ou reordenados falham
// fechado e a string precisa ser a serialização canônica sem whitespace extra.
func (s *Server) requestedCapabilities(scope string) (capabilities, bool) {
	parts := strings.Fields(scope)
	if len(parts) == 0 || strings.Join(parts, " ") != scope || parts[0] != s.config.Scope {
		return capabilities{}, false
	}
	configured := make([]string, 0, 5)
	configured = append(configured, s.config.Scope)
	if s.config.ReadScope != "" {
		configured = append(configured, s.config.ReadScope)
	}
	if s.config.WriteScope != "" {
		configured = append(configured, s.config.WriteScope)
	}
	if s.config.GitScope != "" {
		configured = append(configured, s.config.GitScope)
	}
	if s.config.TestScope != "" {
		configured = append(configured, s.config.TestScope)
	}
	if len(parts) > len(configured) {
		return capabilities{}, false
	}
	last := 0
	result := capabilities{}
	for index, part := range parts {
		position := -1
		for candidate, allowed := range configured {
			if allowed == part {
				position = candidate
				break
			}
		}
		if position < 0 || (index > 0 && position <= last) {
			return capabilities{}, false
		}
		last = position
		switch part {
		case s.config.ReadScope:
			result.Read = true
		case s.config.WriteScope:
			result.Write = true
		case s.config.GitScope:
			result.Git = true
		case s.config.TestScope:
			result.Test = true
		}
	}
	return result, true
}

func (s *Server) readRequested(scope string) bool {
	requested, _ := s.requestedCapabilities(scope)
	return requested.Read
}

func (s *Server) readAllowed(clientID string) bool {
	return s.config.CanIssueRead != nil && s.config.CanIssueRead(clientID)
}

func (s *Server) writeAllowed(clientID string) bool {
	return s.config.CanIssueWrite != nil && s.config.CanIssueWrite(clientID)
}

func (s *Server) gitAllowed(clientID string) bool {
	return s.config.CanIssueGit != nil && s.config.CanIssueGit(clientID)
}

func (s *Server) testAllowed(clientID string) bool {
	return s.config.CanIssueTest != nil && s.config.CanIssueTest(clientID)
}
func (s *Server) jwks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	e := s.key.PublicKey.E
	b := []byte{}
	for e > 0 {
		b = append([]byte{byte(e)}, b...)
		e >>= 8
	}
	jsonReply(w, 200, map[string]any{"keys": []any{map[string]string{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": s.keyID, "n": base64.RawURLEncoding.EncodeToString(s.key.PublicKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(b)}}})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	if !hasContentType(r, "application/json") {
		bad(w, 415, "invalid_request")
		return
	}
	if r.ContentLength > maxRegistrationBytes {
		w.WriteHeader(413)
		return
	}
	var req struct {
		Name          string   `json:"client_name"`
		Redirects     []string `json:"redirect_uris"`
		GrantTypes    []string `json:"grant_types"`
		ResponseTypes []string `json:"response_types"`
		AuthMethod    string   `json:"token_endpoint_auth_method"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRegistrationBytes))
	// Metadados opcionais DCR podem ser enviados pelo cliente; validar os campos usados.
	if dec.Decode(&req) != nil {
		s.rejectRegistration(w, "invalid_client_metadata", "malformed_json")
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		s.rejectRegistration(w, "invalid_client_metadata", "multiple_json_values")
		return
	}
	// RFC 7591 torna client_name opcional; nunca atribuir o nome ChatGPT por conta própria.
	if req.Name == "" {
		req.Name = "Cliente sem nome informado"
	}
	if len(req.Name) > 100 || strings.TrimSpace(req.Name) != req.Name || strings.IndexFunc(req.Name, unicode.IsControl) >= 0 {
		s.rejectRegistration(w, "invalid_client_metadata", "invalid_client_name")
		return
	}
	if len(req.Redirects) < 1 || len(req.Redirects) > 5 {
		s.rejectRegistration(w, "invalid_client_metadata", "invalid_redirect_uris")
		return
	}
	if req.AuthMethod != "" && req.AuthMethod != "none" {
		s.rejectRegistration(w, "invalid_client_metadata", "unsupported_token_auth_method")
		return
	}
	if !supportedRegistrationGrants(req.GrantTypes) {
		s.rejectRegistration(w, "invalid_client_metadata", "unsupported_grant_types")
		return
	}
	if len(req.ResponseTypes) > 0 && (len(req.ResponseTypes) != 1 || req.ResponseTypes[0] != "code") {
		s.rejectRegistration(w, "invalid_client_metadata", "unsupported_response_types")
		return
	}
	for _, u := range req.Redirects {
		if len(u) > 2048 || !allowedRedirect(u) {
			s.rejectRegistration(w, "invalid_redirect_uri", "redirect_uri_not_allowed")
			return
		}
	}
	id, err := randomID(24)
	if err != nil {
		bad(w, 503, "server_error")
		return
	}
	s.mu.Lock()
	if len(s.clients) >= maxClients {
		s.mu.Unlock()
		bad(w, 503, "temporarily_unavailable")
		return
	}
	// Persistir antes de disponibilizar um ID; falha de disco nunca concede registro.
	clients := make(map[string]client, len(s.clients)+1)
	for key, existing := range s.clients {
		clients[key] = existing
	}
	clients[id] = client{Name: req.Name, Redirects: append([]string(nil), req.Redirects...)}
	if err := s.store.save(s.config, s.key, s.keyID, clients); err != nil {
		s.mu.Unlock()
		bad(w, 503, "temporarily_unavailable")
		return
	}
	s.clients = clients
	s.mu.Unlock()
	jsonReply(w, 201, map[string]any{"client_id": id, "client_id_issued_at": time.Now().Unix(), "client_name": req.Name, "redirect_uris": req.Redirects, "grant_types": []string{"authorization_code"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none"})
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	q := r.URL.Query()
	for _, key := range []string{"client_id", "redirect_uri", "response_type", "scope", "resource", "code_challenge", "code_challenge_method", "state"} {
		if len(q[key]) != 1 {
			bad(w, 400, "invalid_request")
			return
		}
	}
	requestedScope := q.Get("scope")
	requested, supported := s.requestedCapabilities(requestedScope)
	if len(r.URL.RawQuery) > maxFormBytes || q.Get("response_type") != "code" || !supported || q.Get("resource") != s.config.ResourceURL || !pkceChallenge.MatchString(q.Get("code_challenge")) || q.Get("code_challenge_method") != "S256" || len(q.Get("state")) < 16 || len(q.Get("state")) > 512 {
		bad(w, 400, "invalid_request")
		return
	}
	id := q.Get("client_id")
	redirect := q.Get("redirect_uri")
	s.mu.Lock()
	c, ok := s.clients[id]
	s.mu.Unlock()
	if !ok {
		bad(w, 400, "invalid_client")
		return
	}
	allowed := false
	for _, u := range c.Redirects {
		if u == redirect {
			allowed = true
			break
		}
	}
	if !allowed {
		bad(w, 400, "invalid_redirect_uri")
		return
	}
	if (requested.Read && !s.readAllowed(id)) || (requested.Write && !s.writeAllowed(id)) || (requested.Git && !s.gitAllowed(id)) || (requested.Test && !s.testAllowed(id)) {
		bad(w, 403, "access_denied")
		return
	}
	pendingID, err := randomID(16)
	if err != nil {
		bad(w, 503, "server_error")
		return
	}
	session, err := randomID(32)
	if err != nil {
		bad(w, 503, "server_error")
		return
	}
	csrf, err := randomID(32)
	if err != nil {
		bad(w, 503, "server_error")
		return
	}
	now := time.Now()
	p := pending{ClientID: id, Redirect: redirect, Challenge: q.Get("code_challenge"), State: q.Get("state"), Resource: s.config.ResourceURL, Scope: requestedScope, SessionHash: sha256.Sum256([]byte(session)), CSRF: csrf, CreatedAt: now, Version: 1, Expires: now.Add(pendingTTL)}
	s.mu.Lock()
	s.clean(time.Now())
	if len(s.pending) >= maxPending {
		s.mu.Unlock()
		bad(w, 503, "temporarily_unavailable")
		return
	}
	s.pending[pendingID] = p
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "signalspace_auth", Value: session, Path: "/authorize", MaxAge: int(pendingTTL.Seconds()), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; connect-src 'self'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if s.config.OnRequest != nil {
		s.config.OnRequest(RequestInfo{ID: pendingID, Client: c.Name, ClientID: id, Redirect: redirect, Scope: requestedScope})
	}
	_ = consentPage.Execute(w, struct {
		ID, Client, ClientID, Redirect, CSRF, Scope string
		Read, Write, Test, Git                      bool
	}{pendingID, c.Name, id, redirect, csrf, requestedScope, requested.Read, requested.Write, requested.Test, requested.Git})
}
func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if r.ParseForm() != nil {
		bad(w, 400, "invalid_request")
		return
	}
	id := r.PostForm.Get("request")
	cookie, err := r.Cookie("signalspace_auth")
	if err != nil || !requestID.MatchString(id) {
		bad(w, 403, "access_denied")
		return
	}
	s.mu.Lock()
	s.clean(time.Now())
	p, ok := s.pending[id]
	if !ok || subtle.ConstantTimeCompare(p.SessionHash[:], hashSession(cookie.Value)) != 1 || subtle.ConstantTimeCompare([]byte(p.CSRF), []byte(r.PostForm.Get("csrf"))) != 1 {
		s.mu.Unlock()
		bad(w, 403, "access_denied")
		return
	}
	if !p.Approved && !p.Denied {
		s.mu.Unlock()
		bad(w, 409, "authorization_pending")
		return
	}
	if p.Denied {
		s.terminalizeLocked(id, p, "DENIED", time.Now())
		delete(s.pending, id)
		s.mu.Unlock()
		bad(w, 403, "access_denied")
		return
	}
	requested, supported := s.requestedCapabilities(p.Scope)
	if !supported || (requested.Read && !s.readAllowed(p.ClientID)) || (requested.Write && !s.writeAllowed(p.ClientID)) || (requested.Git && !s.gitAllowed(p.ClientID)) || (requested.Test && !s.testAllowed(p.ClientID)) {
		s.mu.Unlock()
		bad(w, 403, "access_denied")
		return
	}
	if len(s.codes) >= maxCodes {
		s.mu.Unlock()
		bad(w, 503, "temporarily_unavailable")
		return
	}
	code, err := randomID(32)
	if err != nil {
		s.mu.Unlock()
		bad(w, 503, "server_error")
		return
	}
	// Somente depois de todas as verificações a conclusão consome o pedido.
	s.codes[code] = grant{ClientID: p.ClientID, Redirect: p.Redirect, Challenge: p.Challenge, Resource: p.Resource, Scope: p.Scope, Expires: time.Now().Add(codeTTL)}
	s.terminalizeLocked(id, p, "COMPLETED", time.Now())
	delete(s.pending, id)
	s.mu.Unlock()
	redirect, err := url.Parse(p.Redirect)
	if err != nil {
		bad(w, 500, "server_error")
		return
	}
	q := redirect.Query()
	q.Set("code", code)
	q.Set("state", p.State)
	q.Set("iss", s.config.Issuer)
	redirect.RawQuery = q.Encode()
	http.SetCookie(w, &http.Cookie{Name: "signalspace_auth", Value: "", Path: "/authorize", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, redirect.String(), http.StatusSeeOther)
}
func hashSession(value string) []byte { h := sha256.Sum256([]byte(value)); return h[:] }
func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	if r.Header.Get("Authorization") != "" {
		bad(w, 401, "invalid_client")
		return
	}
	if !hasContentType(r, "application/x-www-form-urlencoded") {
		bad(w, 415, "invalid_request")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if r.ParseForm() != nil {
		bad(w, 400, "invalid_request")
		return
	}
	f := r.PostForm
	for _, key := range []string{"grant_type", "resource", "code", "code_verifier", "client_id", "redirect_uri"} {
		if len(f[key]) != 1 {
			bad(w, 400, "invalid_request")
			return
		}
	}
	if f.Get("grant_type") != "authorization_code" || !pkceVerifier.MatchString(f.Get("code_verifier")) {
		bad(w, 400, "invalid_grant")
		return
	}
	code := f.Get("code")
	s.mu.Lock()
	s.clean(time.Now())
	g, ok := s.codes[code]
	if ok {
		delete(s.codes, code)
	}
	s.mu.Unlock()
	if !ok || g.ClientID != f.Get("client_id") || g.Redirect != f.Get("redirect_uri") || g.Resource != f.Get("resource") {
		bad(w, 400, "invalid_grant")
		return
	}
	digest := sha256.Sum256([]byte(f.Get("code_verifier")))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	if subtle.ConstantTimeCompare([]byte(challenge), []byte(g.Challenge)) != 1 {
		bad(w, 400, "invalid_grant")
		return
	}
	requested, supported := s.requestedCapabilities(g.Scope)
	if !supported || (requested.Read && !s.readAllowed(g.ClientID)) || (requested.Write && !s.writeAllowed(g.ClientID)) || (requested.Git && !s.gitAllowed(g.ClientID)) || (requested.Test && !s.testAllowed(g.ClientID)) {
		bad(w, 400, "invalid_grant")
		return
	}
	now := time.Now()
	jti, err := randomID(16)
	if err != nil {
		bad(w, 503, "server_error")
		return
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": s.keyID})
	// Vincular o token ao cliente registrado que recebeu o código OAuth.
	// O identificador não é uma atestação de que o aplicativo é o ChatGPT.
	payload, _ := json.Marshal(map[string]any{"iss": s.config.Issuer, "sub": ownerSubject, "aud": s.config.ResourceURL, "exp": now.Add(tokenTTL).Unix(), "iat": now.Unix(), "nbf": now.Unix(), "scope": g.Scope, "client_id": g.ClientID, "jti": jti})
	signed := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	h := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, h[:])
	if err != nil {
		bad(w, 503, "server_error")
		return
	}
	jwt := signed + "." + base64.RawURLEncoding.EncodeToString(sig)
	s.mu.Lock()
	s.issued[g.ClientID] = true
	s.mu.Unlock()
	jsonReply(w, 200, map[string]any{"access_token": jwt, "token_type": "Bearer", "expires_in": int(tokenTTL.Seconds()), "scope": g.Scope})
}
