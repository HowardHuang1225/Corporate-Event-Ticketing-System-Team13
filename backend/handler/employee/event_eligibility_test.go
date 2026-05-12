package employee

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestEmployeeEventEligibility(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試活動報名資格會依狀態、截止時間與廠區限制回傳結果",
			Target:      CheckEventEligibilityReturnsExpectedDecisions,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func CheckEventEligibilityReturnsExpectedDecisions(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("活動報名資格：確認系統會依活動狀態、報名截止時間與廠區限制判斷是否可報名。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備活動報名資格測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	now := time.Now().UTC().Truncate(time.Second)
	fixtures := []struct {
		name         string
		status       string
		region       string
		deadline     time.Time
		wantEligible bool
		progress     string
	}{
		{
			name:         "符合報名資格的已發布活動",
			status:       "published",
			region:       "Tainan",
			deadline:     now.Add(24 * time.Hour),
			wantEligible: true,
			progress:     "測試已發布、尚未截止且廠區相符的活動可以報名。",
		},
		{
			name:         "草稿活動不可報名",
			status:       "draft",
			region:       "Tainan",
			deadline:     now.Add(24 * time.Hour),
			wantEligible: false,
			progress:     "測試草稿活動不可報名。",
		},
		{
			name:         "報名截止後不可報名",
			status:       "published",
			region:       "Tainan",
			deadline:     now.Add(-time.Hour),
			wantEligible: false,
			progress:     "測試已發布活動超過報名截止時間後不可報名。",
		},
		{
			name:         "廠區限制不符不可報名",
			status:       "published",
			region:       "Hsinchu",
			deadline:     now.Add(24 * time.Hour),
			wantEligible: false,
			progress:     "測試已發布活動的廠區限制與員工廠區不同時不可報名。",
		},
	}

	router := newEmployeeEventRouter(tx, users.Employee, "employee")
	for _, tt := range fixtures {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("檢查活動報名資格：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("- %s\n", tt.progress))

			event, err := seedEmployeeEligibilityEvent(tx, users.Manager, tt.status, tt.region, tt.deadline)
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/events/"+event.ID.String()+"/eligibility", nil)
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				errs.Add(tt.progress, "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
				return
			}

			body, err := decodeEmployeeEligibilityResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(tt.progress, "%v", err)
				return
			}
			if !body.Success {
				errs.Add(tt.progress, "expected success=true")
				return
			}
			if body.Data.Eligible != tt.wantEligible {
				errs.Add(tt.progress, "expected eligible=%v, got %v", tt.wantEligible, body.Data.Eligible)
				return
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}
