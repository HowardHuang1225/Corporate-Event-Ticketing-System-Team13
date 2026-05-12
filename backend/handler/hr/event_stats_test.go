package hr

import (
	"fmt"
	"net/http"
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestHRReportEventStats(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "HR 可以查看單一活動的報名統計、核銷率與分組資料",
			Target:      HRReportEventStatsReturnsRegistrationAndCheckinMetrics,
		},
		{
			Description: "HR 查詢不存在的活動統計時會收到 NOT_FOUND 錯誤",
			Target:      HRReportEventStatsRejectsMissingEvent,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func HRReportEventStatsReturnsRegistrationAndCheckinMetrics(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("HR 單一活動統計：驗證報名總數、核准數、核銷率與分組統計。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupHRReportTest(t)
	if err != nil {
		errs.Add("準備 HR 單一活動統計測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	fixture, err := seedHRReportMetricsFixture(tx, users)
	if err != nil {
		errs.Add("建立 HR 單一活動統計測試資料", "%v", err)
		return
	}

	router := newHRReportRouter(tx, users.HR)
	resp := performHRGet(router, "/reports/events/"+fixture.Event.ID.String()+"/stats")
	if resp.Code != http.StatusOK {
		errs.Add("呼叫 HR 單一活動統計 API", "預期狀態碼 200，實際為 %d，回應內容 %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeHREventStatsResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析 HR 單一活動統計回應", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析 HR 單一活動統計回應", "預期 success=true")
		return
	}
	if body.Data.Event.ID != fixture.Event.ID {
		errs.Add("檢查 HR 單一活動統計的活動識別", "預期活動 ID %q，實際為 %q", fixture.Event.ID, body.Data.Event.ID)
		return
	}

	tests := []struct {
		name     string
		progress string
		check    func()
	}{
		{
			name:     "報名摘要",
			progress: "摘要需包含所有報名筆數、申請票數、核准票數與取消票數",
			check: func() {
				if body.Data.AppliedApps != 4 {
					errs.Add("檢查 HR 報名摘要", "預期報名筆數為 4，實際為 %d", body.Data.AppliedApps)
				}
				if body.Data.AppliedTickets != 7 {
					errs.Add("檢查 HR 報名摘要", "預期申請票數為 7，實際為 %d", body.Data.AppliedTickets)
				}
				if body.Data.AppliedUsers != 3 {
					errs.Add("檢查 HR 報名摘要", "預期報名人數為 3，實際為 %d", body.Data.AppliedUsers)
				}
				if body.Data.ApprovedApps != 2 {
					errs.Add("檢查 HR 報名摘要", "預期核准報名筆數為 2，實際為 %d", body.Data.ApprovedApps)
				}
				if body.Data.ApprovedTickets != 3 {
					errs.Add("檢查 HR 報名摘要", "預期核准票數為 3，實際為 %d", body.Data.ApprovedTickets)
				}
				if body.Data.ApprovedUsers != 2 {
					errs.Add("檢查 HR 報名摘要", "預期核准人數為 2，實際為 %d", body.Data.ApprovedUsers)
				}
				if body.Data.CancelledTickets != 1 {
					errs.Add("檢查 HR 報名摘要", "預期取消票數為 1，實際為 %d", body.Data.CancelledTickets)
				}
			},
		},
		{
			name:     "核銷摘要",
			progress: "核銷統計需計算已發票券、已核銷票券、已核銷人數與核銷率",
			check: func() {
				if body.Data.TotalTickets != 3 {
					errs.Add("檢查 HR 核銷摘要", "預期已發票券為 3，實際為 %d", body.Data.TotalTickets)
				}
				if body.Data.CheckedInTickets != 2 {
					errs.Add("檢查 HR 核銷摘要", "預期已核銷票券為 2，實際為 %d", body.Data.CheckedInTickets)
				}
				if body.Data.CheckedInUsers != 2 {
					errs.Add("檢查 HR 核銷摘要", "預期已核銷人數為 2，實際為 %d", body.Data.CheckedInUsers)
				}
				if !assertHRFloatEquals(body.Data.CheckInRate, 66.6666666667) {
					errs.Add("檢查 HR 核銷摘要", "預期核銷率約為 66.6667，實際為 %.6f", body.Data.CheckInRate)
				}
			},
		},
		{
			name:     "部門與地區分組",
			progress: "核准使用者需依部門與地區分組，供 HR 分析",
			check: func() {
				if count, ok := hrDeptCount(body.Data.ByDepartment, "業務部"); !ok || count != 2 {
					errs.Add("檢查 HR 部門分組", "預期業務部人數為 2，實際為 %d（是否找到=%v）", count, ok)
				}
				if _, ok := hrDeptCount(body.Data.ByDepartment, "工程部"); ok {
					errs.Add("檢查 HR 部門分組", "預期待審的工程部使用者不應被納入")
				}
				if count, ok := hrRegionCount(body.Data.ByRegion, "北區"); !ok || count != 2 {
					errs.Add("檢查 HR 地區分組", "預期北區人數為 2，實際為 %d（是否找到=%v）", count, ok)
				}
				if _, ok := hrRegionCount(body.Data.ByRegion, "南區"); ok {
					errs.Add("檢查 HR 地區分組", "預期待審的南區使用者不應被納入")
				}
			},
		},
		{
			name:     "票種分組",
			progress: "每個票種需回報申請、核准、取消與有效票券數",
			check: func() {
				general, ok := hrTicketTypeStat(body.Data.ByTicketType, "一般票")
				if !ok {
					errs.Add("檢查 HR 票種分組", "預期回傳一般票統計")
				} else {
					if general.Total != 6 || general.Approved != 2 || general.Cancelled != 1 || general.Active != 2 {
						errs.Add("檢查 HR 票種分組", "預期一般票申請=6 核准=2 取消=1 有效票券=2，實際申請=%d 核准=%d 取消=%d 有效票券=%d", general.Total, general.Approved, general.Cancelled, general.Active)
					}
				}

				vip, ok := hrTicketTypeStat(body.Data.ByTicketType, "VIP票")
				if !ok {
					errs.Add("檢查 HR 票種分組", "預期回傳 VIP 票統計")
				} else {
					if vip.Total != 1 || vip.Approved != 1 || vip.Cancelled != 0 || vip.Active != 1 {
						errs.Add("檢查 HR 票種分組", "預期 VIP 票申請=1 核准=1 取消=0 有效票券=1，實際申請=%d 核准=%d 取消=%d 有效票券=%d", vip.Total, vip.Approved, vip.Cancelled, vip.Active)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("檢查 HR 單一活動統計：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("- %s\n", tt.progress))
			tt.check()
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func HRReportEventStatsRejectsMissingEvent(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("HR 單一活動統計錯誤情境：查詢不存在活動時回傳 NOT_FOUND。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupHRReportTest(t)
	if err != nil {
		errs.Add("準備 HR 不存在活動統計測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	router := newHRReportRouter(tx, users.HR)
	resp := performHRGet(router, "/reports/events/"+uuid.New().String()+"/stats")
	if resp.Code != http.StatusNotFound {
		errs.Add("呼叫不存在活動的 HR 統計 API", "預期狀態碼 404，實際為 %d，回應內容 %s", resp.Code, resp.Body.String())
		return
	}
	if err := utils.AssertHandlerErrorCode(resp.Body.Bytes(), "NOT_FOUND"); err != nil {
		errs.Add("呼叫不存在活動的 HR 統計 API", "%v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}
