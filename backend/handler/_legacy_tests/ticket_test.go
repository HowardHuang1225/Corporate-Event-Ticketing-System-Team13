package handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTicket(t *testing.T) {
	tasks := []testTask{
		{
			description: "測試員工只有符合活動資格時才能送出訂票申請",
			target:      ApplyRequiresEligibleEmployee,
		},
		{
			description: "測試送出申請單時 quantity 必須是正整數",
			target:      ApplyRejectsInvalidQuantity,
		},
		{
			description: "測試送出申請單時 status 必須是合法的數值",
			target:      ApplyRejectsInvalidStatus,
		},
		{
			description: "測試 quantity 不能超過個人上限與剩餘票量",
			target:      ApplyRejectsQuantityBeyondLimits,
		},
	}

	mainTestFunc(t, tasks)
}

func ApplyRequiresEligibleEmployee(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試員工只有符合活動資格時才能送出訂票申請\n")
	printTestProgress("==================================================\n")

	db, err := openTicketTestDB(t)
	if err != nil {
		errs.Add("open ticket test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin ticket test transaction: %v", tx.Error)
		return
	}
	t.Cleanup(func() {
		tx.Rollback()
	})

	redisClient, err := openTicketTestRedis(t)
	if err != nil {
		errs.Add("open ticket test redis", "%v", err)
		return
	}

	manager, err := seedTicketTestUser(tx, "TICKETMGR", "Tainan", "event_manager")
	if err != nil {
		errs.Add("seed manager", "%v", err)
		return
	}
	eligibleEmployee, err := seedTicketTestUser(tx, "TICKETEMP", "Tainan", "employee")
	if err != nil {
		errs.Add("seed eligible employee", "%v", err)
		return
	}
	ineligibleEmployee, err := seedTicketTestUser(tx, "TICKETEMP", "Hsinchu", "employee")
	if err != nil {
		errs.Add("seed ineligible employee", "%v", err)
		return
	}

	region := "Tainan"
	event, ticketType, err := seedTicketApplyEvent(tx, manager, &region, 2, 20)
	if err != nil {
		errs.Add("seed region restricted event", "%v", err)
		return
	}
	resetTicketRedisKeys(t, redisClient, ticketType.ID.String())

	eligibleRouter := newTicketApplyRouter(tx, redisClient, eligibleEmployee)
	eligibleResp := performTicketJSON(eligibleRouter, http.MethodPost, "/applications", validApplyPayload(event, ticketType, 1))
	if eligibleResp.Code != http.StatusCreated {
		errs.Add("finish applying with eligible employee", "expected status 201, got %d with body %s", eligibleResp.Code, eligibleResp.Body.String())
		return
	}
	eligibleBody, err := decodeApplyResponse(eligibleResp.Body.Bytes())
	if err != nil {
		errs.Add("decode eligible apply response", "%v", err)
		return
	}
	if !eligibleBody.Success {
		errs.Add("check eligible apply response", "expected success=true")
		return
	}
	if eligibleBody.Data.UserID != eligibleEmployee.ID {
		errs.Add("check eligible apply response", "expected user_id %q, got %q", eligibleEmployee.ID, eligibleBody.Data.UserID)
		return
	}
	if eligibleBody.Data.Quantity != 1 {
		errs.Add("check eligible apply response", "expected quantity 1, got %d", eligibleBody.Data.Quantity)
		return
	}

	ineligibleRouter := newTicketApplyRouter(tx, redisClient, ineligibleEmployee)
	ineligibleResp := performTicketJSON(ineligibleRouter, http.MethodPost, "/applications", validApplyPayload(event, ticketType, 1))
	if ineligibleResp.Code != http.StatusForbidden {
		errs.Add("finish applying with ineligible employee", "expected status 403, got %d with body %s", ineligibleResp.Code, ineligibleResp.Body.String())
		return
	}
	if err := assertHandlerErrorCode(ineligibleResp.Body.Bytes(), "NOT_ELIGIBLE"); err != nil {
		errs.Add("check ineligible apply response", "%v", err)
		return
	}

	if err := assertTicketApplicationCount(tx, event.ID, 1); err != nil {
		errs.Add("count applications after eligibility checks", "%v", err)
		return
	}

	printTestProgress("==================================================\n\n")
}

func ApplyRejectsInvalidQuantity(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試送出申請單時 quantity 必須是正整數\n")
	printTestProgress("==================================================\n")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", newTicketTestUUID())
		c.Set("role", "employee")
		c.Next()
	})
	router.POST("/applications", NewTicketHandler(nil, nil).Apply)

	for _, tt := range buildInvalidApplyQuantityTests() {
		t.Run(tt.name, func(t *testing.T) {
			payload := validApplyPayloadWithIDs(newTicketTestUUID(), newTicketTestUUID(), 1)
			if tt.omitQuantity {
				delete(payload, "quantity")
			} else {
				payload["quantity"] = tt.quantity
			}

			resp := performTicketJSON(router, http.MethodPost, "/applications", payload)
			if resp.Code != http.StatusBadRequest {
				errs.Add(tt.progress, "expected status 400, got %d with body %s", resp.Code, resp.Body.String())
				return
			}
			if err := assertHandlerErrorCode(resp.Body.Bytes(), "VALIDATION_ERROR"); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	printTestProgress("==================================================\n\n")
}

func ApplyRejectsInvalidStatus(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試送出申請單時 status 必須是合法的數值\n")
	printTestProgress("==================================================\n")

	db, err := openTicketTestDB(t)
	if err != nil {
		errs.Add("open ticket test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin ticket test transaction: %v", tx.Error)
		return
	}
	t.Cleanup(func() {
		tx.Rollback()
	})

	redisClient, err := openTicketTestRedis(t)
	if err != nil {
		errs.Add("open ticket test redis", "%v", err)
		return
	}

	manager, err := seedTicketTestUser(tx, "TICKETMGR", "Tainan", "event_manager")
	if err != nil {
		errs.Add("seed manager", "%v", err)
		return
	}
	employee, err := seedTicketTestUser(tx, "TICKETEMP", "Tainan", "employee")
	if err != nil {
		errs.Add("seed employee", "%v", err)
		return
	}

	router := newTicketApplyRouter(tx, redisClient, employee)
	for _, tt := range buildInvalidApplyStatusTests() {
		t.Run(tt.name, func(t *testing.T) {
			event, ticketType, err := seedTicketApplyEvent(tx, manager, nil, 100, 100)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			resetTicketRedisKeys(t, redisClient, ticketType.ID.String())

			payload := validApplyPayload(event, ticketType, 1)
			if tt.omitStatus {
				delete(payload, "status")
			} else {
				payload["status"] = tt.status
			}

			resp := performTicketJSON(router, http.MethodPost, "/applications", payload)
			if resp.Code != http.StatusBadRequest {
				errs.Add(tt.progress, "expected status 400, got %d with body %s", resp.Code, resp.Body.String())
				return
			}
			if err := assertHandlerErrorCode(resp.Body.Bytes(), "VALIDATION_ERROR"); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if err := assertTicketApplicationCount(tx, event.ID, 0); err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
		})
	}

	printTestProgress("==================================================\n\n")
}

func ApplyRejectsQuantityBeyondLimits(t *testing.T, errs *testErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	printTestProgress("測試 quantity 不能超過個人上限與剩餘票量\n")
	printTestProgress("==================================================\n")

	db, err := openTicketTestDB(t)
	if err != nil {
		errs.Add("open ticket test database", "%v", err)
		return
	}
	tx := db.Begin()
	if tx.Error != nil {
		errs.Add("begin transaction", "failed to begin ticket test transaction: %v", tx.Error)
		return
	}
	t.Cleanup(func() {
		tx.Rollback()
	})

	redisClient, err := openTicketTestRedis(t)
	if err != nil {
		errs.Add("open ticket test redis", "%v", err)
		return
	}

	manager, err := seedTicketTestUser(tx, "TICKETMGR", "Tainan", "event_manager")
	if err != nil {
		errs.Add("seed manager", "%v", err)
		return
	}
	employee, err := seedTicketTestUser(tx, "TICKETEMP", "Tainan", "employee")
	if err != nil {
		errs.Add("seed employee", "%v", err)
		return
	}

	allowanceEvent, allowanceTicketType, err := seedTicketApplyEvent(tx, manager, nil, 2, 10)
	if err != nil {
		errs.Add("seed allowance limited event", "%v", err)
		return
	}
	resetTicketRedisKeys(t, redisClient, allowanceTicketType.ID.String())

	router := newTicketApplyRouter(tx, redisClient, employee)
	allowanceResp := performTicketJSON(router, http.MethodPost, "/applications", validApplyPayload(allowanceEvent, allowanceTicketType, 3))
	if allowanceResp.Code != http.StatusBadRequest {
		errs.Add("finish applying beyond per-person allowance", "expected status 400, got %d with body %s", allowanceResp.Code, allowanceResp.Body.String())
		return
	}
	if err := assertHandlerErrorCode(allowanceResp.Body.Bytes(), "EXCEEDS_MAX_TICKETS"); err != nil {
		errs.Add("check per-person allowance response", "%v", err)
		return
	}
	if err := assertTicketTypeRemaining(tx, allowanceTicketType.ID, 10); err != nil {
		errs.Add("check remaining after allowance rejection", "%v", err)
		return
	}

	inventoryEvent, inventoryTicketType, err := seedTicketApplyEvent(tx, manager, nil, 5, 2)
	if err != nil {
		errs.Add("seed inventory limited event", "%v", err)
		return
	}
	resetTicketRedisKeys(t, redisClient, inventoryTicketType.ID.String())

	inventoryResp := performTicketJSON(router, http.MethodPost, "/applications", validApplyPayload(inventoryEvent, inventoryTicketType, 3))
	if inventoryResp.Code != http.StatusConflict {
		errs.Add("finish applying beyond remaining inventory", "expected status 409, got %d with body %s", inventoryResp.Code, inventoryResp.Body.String())
		return
	}
	if err := assertHandlerErrorCode(inventoryResp.Body.Bytes(), "TICKET_SOLD_OUT"); err != nil {
		errs.Add("check inventory rejection response", "%v", err)
		return
	}
	if err := assertTicketTypeRemaining(tx, inventoryTicketType.ID, 2); err != nil {
		errs.Add("check remaining after inventory rejection", "%v", err)
		return
	}

	printTestProgress("==================================================\n\n")
}
