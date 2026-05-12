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
			Description: "generated tickets include UUID QR tokens",
			Target:      GeneratedTicketsHaveUUIDQRToken,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func GeneratedTicketsHaveUUIDQRToken(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("Ticket generation: approving a pending application should create tickets with QR code UUID tokens.\n")
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
		t.Run(fmt.Sprintf("generated-ticket-%d", index+1), func(t *testing.T) {
			progress := fmt.Sprintf("check generated ticket %d identifiers", index+1)
			t.Logf("Checking generated ticket %d has a UUID QR token", index+1)
			utils.PrintTestProgress(fmt.Sprintf("- %s\n", progress))

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
