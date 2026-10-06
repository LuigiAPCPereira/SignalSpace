package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"unicode"
)

const TunnelProgrammingHeader = "X-SignalSpace-Tunnel-Token"

// TunnelProgrammingConfig autentica somente o último hop local do Secure MCP
// Tunnel. A credencial não concede capabilities; ela produz um principal
// estável que ainda passa por grant, policy e approval owner-side.
type TunnelProgrammingConfig struct {
	OwnerSubject             string
	ClientID                 string
	Token                    string
	Port                     int
	ProgrammingAuthorization func() ProgrammingAuthorizer
	OnMCPEvent               func(string, string)
}

// NewTunnelProgrammingHandler compõe a mesma superfície Programming tipada do
// OAuth v2, mas autentica o hop tunnel-client -> SignalSpace com uma credencial
// estática local. O ChatGPT/Tunnel não recebe essa credencial do SignalSpace.
func NewTunnelProgrammingHandler(config TunnelProgrammingConfig, ports ProgrammingPorts) (http.Handler, error) {
	if err := validateTunnelProgrammingPorts(ports); err != nil {
		return nil, err
	}
	if len(config.OwnerSubject) == 0 || len(config.OwnerSubject) > 512 || strings.TrimSpace(config.OwnerSubject) != config.OwnerSubject || strings.IndexFunc(config.OwnerSubject, unicode.IsControl) >= 0 {
		return nil, errors.New("tunnel owner subject must be configured explicitly")
	}
	if !embeddedClientID.MatchString(config.ClientID) {
		return nil, errors.New("tunnel client ID must use the SignalSpace stable client format")
	}
	if !validToken(config.Token) {
		return nil, errors.New("tunnel token must have at least 32 non-whitespace characters")
	}
	if config.Port < 1 || config.Port > 65535 {
		return nil, errors.New("invalid tunnel MCP port")
	}

	secretHash := sha256.Sum256([]byte(config.Token))
	identity := VerifiedIdentity{OwnerSubject: config.OwnerSubject, ClientID: config.ClientID}
	verify := func(context.Context) (VerifiedIdentity, error) { return identity, nil }

	readAccess := &readToolAccess{
		reader: ports.WorkspaceReader, lister: ports.WorkspaceLister,
		statter: ports.WorkspaceStatter, finder: ports.WorkspaceFinder,
		searcher: ports.WorkspaceSearcher, verify: verify, discoverable: true,
	}
	writeAccess := &writeToolAccess{
		writer: ports.WorkspaceWriter, directoryCreator: ports.WorkspaceDirectoryCreator,
		textCreator: ports.WorkspaceTextCreator, textUpdater: ports.WorkspaceTextUpdater,
		copier: ports.WorkspaceCopier, mover: ports.WorkspaceMover,
		fileDeleter: ports.WorkspaceFileDeleter, directoryDeleter: ports.WorkspaceDirectoryDeleter,
		patchApplier: ports.WorkspacePatchApplier, verify: verify, discoverable: true,
	}
	gitAccess := &gitToolAccess{
		reviewer: ports.GitReviewer, statusReader: ports.GitStatusReader,
		verify: verify, discoverable: true,
	}
	gitIndexAccess := &gitIndexToolAccess{
		mutator: ports.GitIndexer, verify: verify, discoverable: true,
	}
	gitCommitAccess := &gitCommitToolAccess{
		committer: ports.GitCommitter, verify: verify, discoverable: true,
	}
	allowedHosts := map[string]bool{
		"127.0.0.1:" + portString(config.Port): true,
		"localhost:" + portString(config.Port): true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHosts[r.Host] {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path != "/mcp" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		values := r.Header.Values(TunnelProgrammingHeader)
		if len(values) != 1 || !tunnelTokenMatches(values[0], secretHash) {
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var authorizer ProgrammingAuthorizer
		if config.ProgrammingAuthorization != nil {
			authorizer = config.ProgrammingAuthorization()
		}
		serveMCP(w, r, "tunnel_programming", config.OnMCPEvent, readAccess, writeAccess, gitAccess, gitIndexAccess, gitCommitAccess, nil, authorizer, identity)
	}), nil
}

func validateTunnelProgrammingPorts(ports ProgrammingPorts) error {
	if ports.WorkspaceReader == nil || ports.WorkspaceLister == nil || ports.WorkspaceStatter == nil ||
		ports.WorkspaceFinder == nil || ports.WorkspaceSearcher == nil || ports.WorkspaceWriter == nil ||
		ports.WorkspaceDirectoryCreator == nil || ports.WorkspaceTextCreator == nil || ports.WorkspaceTextUpdater == nil ||
		ports.WorkspaceCopier == nil || ports.WorkspaceMover == nil || ports.WorkspaceFileDeleter == nil ||
		ports.WorkspaceDirectoryDeleter == nil || ports.WorkspacePatchApplier == nil || ports.GitReviewer == nil ||
		ports.GitStatusReader == nil || ports.GitIndexer == nil || ports.GitCommitter == nil {
		return errors.New("tunnel programming composition requires all typed filesystem and local Git ports")
	}
	return nil
}

func tunnelTokenMatches(value string, expected [32]byte) bool {
	provided := sha256.Sum256([]byte(value))
	return subtle.ConstantTimeCompare(provided[:], expected[:]) == 1
}

func portString(port int) string {
	const digits = "0123456789"
	if port == 0 {
		return "0"
	}
	var buf [5]byte
	index := len(buf)
	for port > 0 {
		index--
		buf[index] = digits[port%10]
		port /= 10
	}
	return string(buf[index:])
}
