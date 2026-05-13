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
			Description: "測試管理者可用掃描 QR code 或手動輸入 UUID token 核銷票券",
			Target:      CheckinAcceptsScannedQRCodeAndManualUUIDToken,
		},
		{
			Description: "測試核銷會拒絕不合法或不存在的識別碼",
			Target:      CheckinRejectsInvalidOrUnknownIdentifiers,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func CheckinAcceptsScannedQRCodeAndManualUUIDToken(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("票券核銷：管理者可掃描 QR code token 或手動輸入 UUID token 完成核銷。\n")
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
			name:     "掃描 QR code",
			progress: "使用產生 QR code 解析出的 UUID token 核銷票券。",
			payload:  gin.H{"qr_token": scannedTicket.QRToken},
			ticket:   scannedTicket.ID,
		},
		{
			name:     "手動輸入 UUID token",
			progress: "手動輸入 QR code 對應的 UUID token 核銷票券。",
			payload:  gin.H{"qr_token": manualTicket.QRToken},
			ticket:   manualTicket.ID,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))

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

	utils.PrintTestProgress("票券核銷錯誤情境：不合法、不存在與已使用的識別碼都應被拒絕。\n")
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
			name:       "缺少識別碼",
			progress:   "送出核銷請求時缺少 qr_token 應被拒絕。",
			payload:    gin.H{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "空白 QR token",
			progress:   "送出核銷請求時 qr_token 為空字串應被拒絕。",
			payload:    gin.H{"qr_token": ""},
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "QR token 格式錯誤",
			progress:   "送出核銷請求時 qr_token 格式錯誤應被拒絕。",
			payload:    gin.H{"qr_token": "not-a-uuid"},
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "不存在的 QR token",
			progress:   "送出核銷請求時使用格式正確但不存在的 UUID token 應被拒絕。",
			payload:    gin.H{"qr_token": uuid.New().String()},
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "票券已核銷",
			progress:   "送出核銷請求時使用已被核銷過的 UUID token 應被拒絕。",
			payload:    gin.H{"qr_token": usedTicket.QRToken},
			wantStatus: http.StatusConflict,
			wantCode:   "ALREADY_CHECKED_IN",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))

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
