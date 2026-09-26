package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/approval"
	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

func capabilityApprovalTestInput(summary string, last byte) approval.RequestInput {
	fingerprint := strings.Repeat("0123456789abcdef", 4)
	fingerprint = fingerprint[:63] + string([]byte{last})
	return approval.RequestInput{
		OwnerID: "owner", ClientID: "client", TokenFamilyID: "family", WorkspaceID: "workspace",
		SessionID: "session", Capability: capability.WorkspaceWrite, Tool: "write_text_file",
		OperationFingerprint: fingerprint, SafeSummary: summary,
	}
}

func TestCapabilityApprovalHTTPIsSeparateAndOwnerProtected(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	manager := approval.New()
	defer manager.Close()
	handler := gate.HandlerWithRequestsAndCapabilityApprovals(nil, manager)

	unauthenticated := adminRequest(handler, http.MethodGet, "/api/admin/v1/capability-approvals", "", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated approval list status = %d", unauthenticated.Code)
	}
	bootstrapResponse := adminRequest(handler, http.MethodGet, "/api/admin/v1/session", "", "")
	bootstrapCookie := responseCookie(t, bootstrapResponse, bootstrapCookie)
	bootstrapCSRF := adminResponse(t, bootstrapResponse)["csrf_token"].(string)
	pairPayload := `{"pairing_code":"` + code + `","passphrase":"` + testPassphrase + `"}`
	paired := adminRequest(handler, http.MethodPost, "/api/admin/v1/pair", pairPayload, bootstrapCSRF, bootstrapCookie)
	if paired.Code != http.StatusCreated {
		t.Fatalf("pair status = %d body=%s", paired.Code, paired.Body.String())
	}
	ownerCookie := responseCookie(t, paired, adminCookie)
	ownerCSRF := adminResponse(t, paired)["csrf_token"].(string)

	request, reused, err := manager.Create(capabilityApprovalTestInput("Write src/main.go", 'a'))
	if err != nil || reused {
		t.Fatalf("create approval request = %+v reused=%t err=%v", request, reused, err)
	}
	listed := adminRequest(handler, http.MethodGet, "/api/admin/v1/capability-approvals", "", "", ownerCookie)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), request.RequestID) {
		t.Fatalf("approval list status/body = %d %s", listed.Code, listed.Body.String())
	}
	detail := adminRequest(handler, http.MethodGet, "/api/admin/v1/capability-approvals/"+request.RequestID, "", "", ownerCookie)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"status":"PENDING"`) {
		t.Fatalf("approval detail status/body = %d %s", detail.Code, detail.Body.String())
	}
	wrongCSRF := adminRequest(handler, http.MethodPost, "/api/admin/v1/capability-approvals/"+request.RequestID+"/decision", `{"expected_version":1,"decision":"ALLOW_ONCE"}`, "wrong", ownerCookie)
	if wrongCSRF.Code != http.StatusForbidden {
		t.Fatalf("wrong csrf status = %d body=%s", wrongCSRF.Code, wrongCSRF.Body.String())
	}
	extraField := adminRequest(handler, http.MethodPost, "/api/admin/v1/capability-approvals/"+request.RequestID+"/decision", `{"expected_version":1,"decision":"ALLOW_ONCE","extra":true}`, ownerCSRF, ownerCookie)
	if extraField.Code != http.StatusBadRequest {
		t.Fatalf("extra field status = %d body=%s", extraField.Code, extraField.Body.String())
	}
	approved := adminRequest(handler, http.MethodPost, "/api/admin/v1/capability-approvals/"+request.RequestID+"/decision", `{"expected_version":1,"decision":"ALLOW_ONCE"}`, ownerCSRF, ownerCookie)
	if approved.Code != http.StatusOK || !strings.Contains(approved.Body.String(), `"status":"APPROVED"`) || strings.Contains(approved.Body.String(), "permit") {
		t.Fatalf("approval decision status/body = %d %s", approved.Code, approved.Body.String())
	}

	second, _, err := manager.Create(capabilityApprovalTestInput("Write src/other.go", 'b'))
	if err != nil {
		t.Fatal(err)
	}
	denied := adminRequest(handler, http.MethodPost, "/api/admin/v1/capability-approvals/"+second.RequestID+"/decision", `{"expected_version":1,"decision":"DENY"}`, ownerCSRF, ownerCookie)
	if denied.Code != http.StatusOK || !strings.Contains(denied.Body.String(), `"status":"DENIED"`) {
		t.Fatalf("deny decision status/body = %d %s", denied.Code, denied.Body.String())
	}
	if _, err := manager.DecideSnapshot(request.RequestID, 1, approval.DecisionDeny); !errors.Is(err, approval.ErrAlreadyDecided) {
		t.Fatalf("approved request changed through manager: %v", err)
	}
}

func TestCapabilityApprovalHTTPRejectsCrossOriginMutation(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	manager := approval.New()
	defer manager.Close()
	handler := gate.HandlerWithRequestsAndCapabilityApprovals(nil, manager)
	bootstrapResponse := adminRequest(handler, http.MethodGet, "/api/admin/v1/session", "", "")
	bootstrapCookie := responseCookie(t, bootstrapResponse, bootstrapCookie)
	bootstrapCSRF := adminResponse(t, bootstrapResponse)["csrf_token"].(string)
	pairPayload := `{"pairing_code":"` + code + `","passphrase":"` + testPassphrase + `"}`
	paired := adminRequest(handler, http.MethodPost, "/api/admin/v1/pair", pairPayload, bootstrapCSRF, bootstrapCookie)
	ownerCookie := responseCookie(t, paired, adminCookie)
	ownerCSRF := adminResponse(t, paired)["csrf_token"].(string)
	request, _, err := manager.Create(capabilityApprovalTestInput("Write src/main.go", 'c'))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/v1/capability-approvals/"+request.RequestID+"/decision", strings.NewReader(`{"expected_version":1,"decision":"DENY"}`))
	req.Host = "localhost:7677"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", ownerCSRF)
	req.AddCookie(ownerCookie)
	req.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin approval mutation status = %d body=%s", response.Code, response.Body.String())
	}
}
