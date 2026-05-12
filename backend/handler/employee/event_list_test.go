package employee

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestEmployeeListEvents(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試活動列表在管理者情境可依各種狀態篩選",
			Target:      ListEventsCanFilterEveryStatusForManagerContext,
		},
		{
			Description: "測試員工看不到 draft 狀態的活動",
			Target:      ListEventsHidesDraftEventsFromEmployeeContext,
		},
		{
			Description: "測試員工可以依狀態、票種與活動時間篩選活動",
			Target:      ListEventsCanFilterByStatusTicketTypeAndTime,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func ListEventsCanFilterByStatusTicketTypeAndTime(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("活動列表：確認員工可以依 status、ticket_type、start_from、start_to 篩選活動。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備活動條件篩選測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	now := time.Now().UTC().Truncate(time.Second)
	titlePrefix := fmt.Sprintf("員工活動條件篩選測試 %s", utils.UniqueTestSuffix())
	target, err := seedEmployeeListFilterEvent(tx, users.Manager, titlePrefix+" 符合條件", "published", "VIP票", now.Add(72*time.Hour))
	if err != nil {
		errs.Add("建立符合條件的活動測試資料", "%v", err)
		return
	}
	if _, err := seedEmployeeListFilterEvent(tx, users.Manager, titlePrefix+" 狀態不符", "draft", "VIP票", now.Add(72*time.Hour)); err != nil {
		errs.Add("建立狀態不符的活動測試資料", "%v", err)
		return
	}
	if _, err := seedEmployeeListFilterEvent(tx, users.Manager, titlePrefix+" 票種不符", "published", "一般票", now.Add(72*time.Hour)); err != nil {
		errs.Add("建立票種不符的活動測試資料", "%v", err)
		return
	}
	if _, err := seedEmployeeListFilterEvent(tx, users.Manager, titlePrefix+" 時間不符", "published", "VIP票", now.Add(240*time.Hour)); err != nil {
		errs.Add("建立時間不符的活動測試資料", "%v", err)
		return
	}

	startFrom := now.Add(48 * time.Hour)
	startTo := now.Add(96 * time.Hour)
	query := url.Values{}
	query.Set("status", "published")
	query.Set("ticket_type", "VIP票")
	query.Set("start_from", startFrom.Format(time.RFC3339))
	query.Set("start_to", startTo.Format(time.RFC3339))

	router := newEmployeeEventRouter(tx, users.Employee, "employee")
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events?"+query.Encode(), nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		errs.Add("依條件查詢活動列表", "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEmployeeEventListResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析活動條件篩選回應", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析活動條件篩選回應", "expected success=true")
		return
	}

	foundTarget := false
	for _, event := range body.Data {
		if event.Title == target.Title {
			foundTarget = true
		}
		if !hasEmployeeEventPrefix(event.Title, titlePrefix) {
			continue
		}
		if event.Title != target.Title {
			errs.Add("檢查活動條件篩選結果", "不符合條件的活動 %q 不應出現在回應中", event.Title)
			return
		}
		if event.Status != "published" {
			errs.Add("檢查活動條件篩選結果", "expected status published, got %q", event.Status)
			return
		}
		if !employeeEventHasTicketType(event, "VIP票") {
			errs.Add("檢查活動條件篩選結果", "回應活動應包含 VIP票 票種")
			return
		}
		if event.StartTime.Before(startFrom) || event.StartTime.After(startTo) {
			errs.Add("檢查活動條件篩選結果", "活動開始時間應介於 %s 與 %s 之間，got %s", startFrom, startTo, event.StartTime)
			return
		}
	}
	if !foundTarget {
		errs.Add("檢查活動條件篩選結果", "活動列表應包含符合條件的活動 %q", target.Title)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ListEventsCanFilterEveryStatusForManagerContext(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("活動列表：確認管理者情境可以依 draft、published、closed、ended 狀態篩選活動。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備活動列表測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	titlePrefix := fmt.Sprintf("管理者活動列表測試 %s", utils.UniqueTestSuffix())
	states := []string{"draft", "published", "closed", "ended"}
	if err := seedEmployeeEventsForStates(tx, users.Manager, titlePrefix, states); err != nil {
		errs.Add("建立各狀態活動測試資料", "%v", err)
		return
	}

	router := newEmployeeEventRouter(tx, users.Manager, "event_manager")
	for _, state := range states {
		t.Run(state, func(t *testing.T) {
			progress := fmt.Sprintf("測試管理者情境可以查詢 %q 狀態的活動。", state)
			t.Log(progress)
			utils.PrintTestProgress(fmt.Sprintf("- %s\n", progress))

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/events?status="+state, nil)
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				errs.Add(progress, "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
				return
			}

			body, err := decodeEmployeeEventListResponse(resp.Body.Bytes())
			if err != nil {
				errs.Add(progress, "%v", err)
				return
			}
			if !body.Success {
				errs.Add(progress, "expected success=true")
				return
			}
			if !employeeEventListContainsStateFixture(body.Data, titlePrefix, state) {
				errs.Add(progress, "活動列表回應應包含事先建立的 %q 狀態活動", state)
				return
			}
			for _, event := range body.Data {
				if hasEmployeeEventPrefix(event.Title, titlePrefix) && event.Status != state {
					errs.Add(progress, "測試資料 %q 狀態不符合預期，expected %q, got %q", event.Title, state, event.Status)
					return
				}
			}
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ListEventsHidesDraftEventsFromEmployeeContext(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("活動列表：確認員工看不到 draft 狀態的活動。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupEmployeeEventTest(t)
	if err != nil {
		errs.Add("準備員工不可見 draft 活動測試資料庫", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	titlePrefix := fmt.Sprintf("員工不可見 draft 活動測試 %s", utils.UniqueTestSuffix())
	states := []string{"draft", "published", "closed", "ended"}
	if err := seedEmployeeEventsForStates(tx, users.Manager, titlePrefix, states); err != nil {
		errs.Add("建立員工不可見 draft 活動測試資料", "%v", err)
		return
	}

	router := newEmployeeEventRouter(tx, users.Employee, "employee")
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		errs.Add("以員工身分查詢活動列表", "expected status 200, got %d with body %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeEmployeeEventListResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析員工活動列表回應", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析員工活動列表回應", "expected success=true")
		return
	}
	if !employeeEventListContainsStateFixture(body.Data, titlePrefix, "published") {
		errs.Add("檢查員工活動列表", "員工應可看見 published 狀態活動")
		return
	}
	if !employeeEventListContainsStateFixture(body.Data, titlePrefix, "closed") {
		errs.Add("檢查員工活動列表", "員工應可看見 closed 狀態活動")
		return
	}
	for _, event := range body.Data {
		if !hasEmployeeEventPrefix(event.Title, titlePrefix) {
			continue
		}
		if event.Status == "draft" {
			errs.Add("檢查員工活動列表", "員工不應看見 draft 狀態活動")
			return
		}
	}

	utils.PrintTestProgress("==================================================\n\n")
}
