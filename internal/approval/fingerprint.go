package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var ErrInvalidFingerprintInput = errors.New("invalid fingerprint input")

// CanonicalFingerprint produz o hash da operação sem armazenar seus
// argumentos. encoding/json ordena chaves de mapas, dando uma representação
// determinística para o material estruturado fornecido pela camada chamadora.
// A versão permite evoluir o formato sem aceitar hashes de contratos
// incompatíveis silenciosamente.
func CanonicalFingerprint(tool, sessionID string, operation any) (string, error) {
	if strings.TrimSpace(tool) != tool || tool == "" || strings.TrimSpace(sessionID) != sessionID || sessionID == "" {
		return "", ErrInvalidFingerprintInput
	}
	material := struct {
		Version   int    `json:"version"`
		Tool      string `json:"tool"`
		SessionID string `json:"session_id"`
		Operation any    `json:"operation"`
	}{Version: 1, Tool: tool, SessionID: sessionID, Operation: operation}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", ErrInvalidFingerprintInput
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
