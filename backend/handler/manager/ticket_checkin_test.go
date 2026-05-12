package manager

import (
	"fmt"
	"net/http"
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestManagerTicketCheckin(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "check in tickets by scanned QR code and manual UUID token",
			Target:      CheckinAcceptsScannedQRCodeAndManualUUIDToken,
		},
		{
			Description: "check-in rejects invalid or unknown identifiers",
			Target:      CheckinRejectsInvalidOrUnknownIdentifiers,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func CheckinAcceptsScannedQRCodeAndManualUUIDToken(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("Ticket check-in: manager can check in tickets by scanning the QR code token or manually entering the UUID token.\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupManagerTicketLifecycleTest(t)
	if err != nil {
		errs.Add("set up ticket check-in test data", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerTicketLifecycleRouter(tx, users.Manager)
	scannedTicket, err := seedManagerCheckinTicket(tx, users, false)
	if err != nil {
		errs.Add("seed the scanned QR code check-in ticket", "%v", err)
		return
	}
	manualTicket, err := seedManagerCheckinTicket(tx, users, false)
	if err != nil {
		errs.Add("seed the manual UUID token check-in ticket", "%v", err)
		return
	}

	tests := []struct {
		name     string
		progress string
		payload  gin.H
		ticket   uuid.UUID
	}{
		{
			name:     "scanned-qr-code",
			progress: "check in with the UUID token decoded from the generated QR code",
			payload:  gin.H{"qr_token": scannedTicket.QRToken},
			ticket:   scannedTicket.ID,
		},
		{
			name:     "manual-uuid-token",
			progress: "check in by manually entering the QR code UUID token",
			payload:  gin.H{"qr_token": manualTicket.QRToken},
			ticket:   manualTicket.ID,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Checking ticket check-in path: %s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("- %s\n", tt.progress))

			token, ok := tt.payload["qr_token"].(string)
			if !ok {
				errs.Add(tt.progress, "expected test payload to contain qr_token")
				return
			}
			if parsed, err := uuid.Parse(token); err != nil || parsed == uuid.Nil {
				errs.Add(tt.progress, "expected qr_token %q to be a valid UUID token", token)
				return
			}

			resp := utils.PerformJSON(router, http.MethodPost, "/checkin", tt.payload)
			if resp.Code != http.StatusOK {
				errs.Add(tt.progress, "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
				return
			}

			body, err := decodeManagerCheckinResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if !body.Success {
				errs.Add(tt.progress, "expected success=true")
				return
			}
			if body.Data.Ticket.ID != tt.ticket {
				errs.Add(tt.progress, "expected ticket id %q, got %q", tt.ticket, body.Data.Ticket.ID)
				return
			}
			if !body.Data.Ticket.IsUsed {
				errs.Add(tt.progress, "expected response ticket to be marked used")
				return
			}

			persisted, err := managerTicketByID(tx, tt.ticket)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if !persisted.IsUsed {
				errs.Add(tt.progress, "expected persisted ticket to be marked used")
				return
			}
			checkins, err := managerTicketCheckinCount(tx, tt.ticket)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if checkins != 1 {
				errs.Add(tt.progress, "expected one check-in record, got %d", checkins)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func CheckinRejectsInvalidOrUnknownIdentifiers(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("Ticket check-in errors: invalid, unknown, and already used identifiers should be rejected.\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupManagerTicketLifecycleTest(t)
	if err != nil {
		errs.Add("set up ticket check-in error test data", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerTicketLifecycleRouter(tx, users.Manager)
	usedTicket, err := seedManagerCheckinTicket(tx, users, true)
	if err != nil {
		errs.Add("seed the already used ticket", "%v", err)
		return
	}

	tests := []struct {
		name       string
		progress   string
		payload    gin.H
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing identifier",
			progress:   "submit check-in without qr_token",
			payload:    gin.H{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "empty qr token",
			progress:   "submit check-in with an empty qr_token",
			payload:    gin.H{"qr_token": ""},
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "invalid qr token",
			progress:   "submit check-in with a malformed qr_token",
			payload:    gin.H{"qr_token": "not-a-uuid"},
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "unknown qr token",
			progress:   "submit check-in with a well-formed but nonexistent UUID token",
			payload:    gin.H{"qr_token": uuid.New().String()},
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "already checked in",
			progress:   "submit check-in with a UUID token that has already been used",
			payload:    gin.H{"qr_token": usedTicket.QRToken},
			wantStatus: http.StatusConflict,
			wantCode:   "ALREADY_CHECKED_IN",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Checking ticket check-in rejection: %s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("- %s\n", tt.progress))

			resp := utils.PerformJSON(router, http.MethodPost, "/checkin", tt.payload)
			if resp.Code != tt.wantStatus {
				errs.Add(tt.progress, "expected status %d, got %d with body %s", tt.wantStatus, resp.Code, resp.Body.String())
				return
			}
			if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), tt.wantCode); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
