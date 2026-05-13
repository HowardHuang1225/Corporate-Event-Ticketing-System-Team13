package manager

import (
	"fmt"
	"net/http"
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestManagerTicketGeneration(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試核准申請後產生的票券包含 UUID QR token",
			Target:      GeneratedTicketsHaveUUIDQRToken,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func GeneratedTicketsHaveUUIDQRToken(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("票券產生：核准 pending 申請後，應建立帶有 QR code UUID token 的票券。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupManagerTicketLifecycleTest(t)
	if err != nil {
		errs.Add("set up ticket generation test data", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newManagerTicketLifecycleRouter(tx, users.Manager)
	_, _, app, err := seedManagerApplicationFixture(tx, users, "pending", 2, 8)
	if err != nil {
		errs.Add("seed a pending application for ticket generation", "%v", err)
		return
	}

	resp := utils.PerformJSON(router, http.MethodPost, "/applications/"+app.ID.String()+"/approve", gin.H{})
	if resp.Code != http.StatusOK {
		errs.Add("approve the pending application", "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeManagerApplicationResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("decode the approve response", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("decode the approve response", "expected success=true")
		return
	}
	if body.Data.Status != "approved" {
		errs.Add("check approved application status", "expected status approved, got %q", body.Data.Status)
		return
	}
	if len(body.Data.Tickets) != app.Quantity {
		errs.Add("check generated tickets in response", "expected %d tickets, got %d", app.Quantity, len(body.Data.Tickets))
		return
	}

	seenTicketIDs := map[string]bool{}
	seenQRTokens := map[string]bool{}
	for index, ticket := range body.Data.Tickets {
		ticket := ticket
		index := index
		t.Run(fmt.Sprintf("產生票券-%d", index+1), func(t *testing.T) {
			progress := fmt.Sprintf("檢查第 %d 張產生票券的識別資料。", index+1)
			t.Logf("子測試：%s", progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", progress))

			if ticket.ID == uuid.Nil {
				errs.Add(progress, "expected ticket id to be a non-empty UUID")
				return
			}
			if _, err := uuid.Parse(ticket.ID.String()); err != nil {
				errs.Add(progress, "expected ticket id %q to parse as UUID: %v", ticket.ID, err)
				return
			}
			if ticket.QRToken == "" {
				errs.Add(progress, "expected qr_token to be present")
				return
			}
			qrUUID, err := uuid.Parse(ticket.QRToken)
			if err != nil || qrUUID == uuid.Nil {
				errs.Add(progress, "expected qr_token %q to be a non-empty UUID", ticket.QRToken)
				return
			}
			if seenTicketIDs[ticket.ID.String()] {
				errs.Add(progress, "expected ticket id %q to be unique", ticket.ID)
				return
			}
			if seenQRTokens[ticket.QRToken] {
				errs.Add(progress, "expected qr_token %q to be unique", ticket.QRToken)
				return
			}
			seenTicketIDs[ticket.ID.String()] = true
			seenQRTokens[ticket.QRToken] = true
			if ticket.ApplicationID != app.ID {
				errs.Add(progress, "expected application_id %q, got %q", app.ID, ticket.ApplicationID)
				return
			}
			if ticket.UserID != users.Employee.ID {
				errs.Add(progress, "expected user_id %q, got %q", users.Employee.ID, ticket.UserID)
				return
			}
			if ticket.ExpiresAt.IsZero() {
				errs.Add(progress, "expected expires_at to be set")
				return
			}
		})
	}

	persisted, err := managerPersistedTicketsForApplication(tx, app.ID)
	if err != nil {
		errs.Add("load generated tickets from database", "%v", err)
		return
	}
	if len(persisted) != app.Quantity {
		errs.Add("check generated tickets in database", "expected %d persisted tickets, got %d", app.Quantity, len(persisted))
		return
	}
	for _, ticket := range persisted {
		if ticket.ID == uuid.Nil || ticket.QRToken == "" {
			errs.Add("check generated tickets in database", "expected persisted ticket %q to keep both id and qr_token", ticket.ID)
			return
		}
	}

	utils.PrintTestProgress("==================================================\n\n")
}
