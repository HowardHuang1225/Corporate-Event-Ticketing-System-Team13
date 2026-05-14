package hr

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
)

func TestHRReportOverview(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "HR 可以查看各活動的報名與核銷統計",
			Target:      HRReportOverviewReturnsMetricsForEachEvent,
		},
		{
			Description: "HR 可以匯出單一活動的報名與核銷統計 CSV",
			Target:      HRReportExportEventCSVIncludesRegistrationAndCheckinMetrics,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func HRReportOverviewReturnsMetricsForEachEvent(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("HR 活動總覽：驗證各活動都有報名總數與核銷率。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupHRReportTest(t)
	if err != nil {
		errs.Add("準備 HR 活動總覽測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	metricsFixture, err := seedHRReportMetricsFixture(tx, users)
	if err != nil {
		errs.Add("建立 HR 活動總覽統計測試資料", "%v", err)
		return
	}
	pendingOnlyEvent, err := seedHRReportPendingOnlyEvent(tx, users)
	if err != nil {
		errs.Add("建立 HR 活動總覽僅待審測試資料", "%v", err)
		return
	}

	router := newHRReportRouter(tx, users.HR)
	resp := performHRGet(router, "/reports/overview")
	if resp.Code != http.StatusOK {
		errs.Add("呼叫 HR 活動總覽 API", "預期狀態碼 200，實際為 %d，回應內容 %s", resp.Code, resp.Body.String())
		return
	}

	body, err := decodeHROverviewResponse(resp.Body.Bytes())
	if err != nil {
		errs.Add("解析 HR 活動總覽回應", "%v", err)
		return
	}
	if !body.Success {
		errs.Add("解析 HR 活動總覽回應", "預期 success=true")
		return
	}

	tests := []struct {
		name     string
		progress string
		check    func()
	}{
		{
			name:     "已有核銷的活動",
			progress: "已有核銷活動的總覽列需包含申請、核准、取消、發票與核銷統計",
			check: func() {
				row, ok := hrOverviewForEvent(body.Data, metricsFixture.Event.ID.String())
				if !ok {
					errs.Add("檢查 HR 總覽中的已核銷活動", "預期總覽包含活動 %q", metricsFixture.Event.ID)
					return
				}
				if row.Title != metricsFixture.Event.Title {
					errs.Add("檢查 HR 總覽中的已核銷活動", "預期標題 %q，實際為 %q", metricsFixture.Event.Title, row.Title)
				}
				if row.AppliedApps != 4 || row.AppliedTickets != 7 || row.AppliedUsers != 3 {
					errs.Add("檢查 HR 總覽中的已核銷活動", "預期申請筆數=4 申請票數=7 申請人數=3，實際筆數=%d 票數=%d 人數=%d", row.AppliedApps, row.AppliedTickets, row.AppliedUsers)
				}
				if row.ApprovedTickets != 3 || row.ApprovedUsers != 2 || row.Cancelled != 1 {
					errs.Add("檢查 HR 總覽中的已核銷活動", "預期核准票數=3 核准人數=2 取消票數=1，實際核准票數=%d 核准人數=%d 取消票數=%d", row.ApprovedTickets, row.ApprovedUsers, row.Cancelled)
				}
				if row.TotalTickets != 3 || row.CheckedIn != 2 {
					errs.Add("檢查 HR 總覽中的已核銷活動", "預期已發票券=3 已核銷=2，實際已發票券=%d 已核銷=%d", row.TotalTickets, row.CheckedIn)
				}
				if !assertHRFloatEquals(row.CheckInRate, 66.6666666667) {
					errs.Add("檢查 HR 總覽中的已核銷活動", "預期核銷率約為 66.6667，實際為 %.6f", row.CheckInRate)
				}
			},
		},
		{
			name:     "尚未發票的活動",
			progress: "只有待審報名且尚未發票的活動，核銷率需維持 0",
			check: func() {
				row, ok := hrOverviewForEvent(body.Data, pendingOnlyEvent.ID.String())
				if !ok {
					errs.Add("檢查 HR 總覽中的僅待審活動", "預期總覽包含活動 %q", pendingOnlyEvent.ID)
					return
				}
				if row.AppliedApps != 1 || row.AppliedTickets != 4 || row.AppliedUsers != 1 {
					errs.Add("檢查 HR 總覽中的僅待審活動", "預期申請筆數=1 申請票數=4 申請人數=1，實際筆數=%d 票數=%d 人數=%d", row.AppliedApps, row.AppliedTickets, row.AppliedUsers)
				}
				if row.ApprovedTickets != 0 || row.ApprovedUsers != 0 || row.Cancelled != 0 {
					errs.Add("檢查 HR 總覽中的僅待審活動", "預期沒有核准或取消數量，實際核准票數=%d 核准人數=%d 取消票數=%d", row.ApprovedTickets, row.ApprovedUsers, row.Cancelled)
				}
				if row.TotalTickets != 0 || row.CheckedIn != 0 || row.CheckInRate != 0 {
					errs.Add("檢查 HR 總覽中的僅待審活動", "預期沒有發票與核銷資料，實際已發票券=%d 已核銷=%d 核銷率=%.6f", row.TotalTickets, row.CheckedIn, row.CheckInRate)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))
			tt.check()
		})
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func HRReportExportEventCSVIncludesRegistrationAndCheckinMetrics(t *testing.T, errs *utils.Errors) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	utils.PrintTestProgress("HR CSV 匯出：驗證單一活動的報名與核銷統計內容正確。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupHRReportTest(t)
	if err != nil {
		errs.Add("準備 HR CSV 匯出測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	fixture, err := seedHRReportMetricsFixture(tx, users)
	if err != nil {
		errs.Add("建立 HR CSV 匯出測試資料", "%v", err)
		return
	}

	router := newHRReportRouter(tx, users.HR)
	resp := performHRGet(router, "/reports/events/"+fixture.Event.ID.String()+"/export")
	if resp.Code != http.StatusOK {
		errs.Add("呼叫 HR CSV 匯出 API", "預期狀態碼 200，實際為 %d，回應內容 %s", resp.Code, resp.Body.String())
		return
	}
	if contentType := resp.Header().Get("Content-Type"); contentType != "text/csv" {
		errs.Add("檢查 HR CSV 匯出標頭", "預期 Content-Type 為 text/csv，實際為 %q", contentType)
		return
	}
	if disposition := resp.Header().Get("Content-Disposition"); disposition != "attachment; filename=event-stats.csv" {
		errs.Add("檢查 HR CSV 匯出標頭", "預期 attachment disposition，實際為 %q", disposition)
		return
	}

	metrics, err := readHRMetricCSV(resp.Body.String())
	if err != nil {
		errs.Add("解析 HR CSV 匯出內容", "%v", err)
		return
	}
	if len(metrics) != 11 {
		errs.Add("檢查 HR CSV 匯出內容", "預期 CSV 有 11 個 metric，實際為 %d：%v", len(metrics), metrics)
	}

	tests := []struct {
		name     string
		progress string
		metric   string
		want     string
	}{
		{name: "報名筆數", progress: "CSV 需包含報名筆數", metric: "applied_apps", want: "4"},
		{name: "申請票數", progress: "CSV 需包含申請票數", metric: "applied_tickets", want: "7"},
		{name: "報名人數", progress: "CSV 需包含報名人數", metric: "applied_users", want: "3"},
		{name: "核准筆數", progress: "CSV 需包含核准報名筆數", metric: "approved_apps", want: "2"},
		{name: "核准票數", progress: "CSV 需包含核准票數", metric: "approved_tickets", want: "3"},
		{name: "核准人數", progress: "CSV 需包含核准人數", metric: "approved_users", want: "2"},
		{name: "取消票數", progress: "CSV 需包含取消票數", metric: "cancelled_tickets", want: "1"},
		{name: "已發票券", progress: "CSV 需包含已發票券數", metric: "total_tickets", want: "3"},
		{name: "已核銷票券", progress: "CSV 需包含已核銷票券數", metric: "checked_in_tickets", want: "2"},
		{name: "已核銷人數", progress: "CSV 需包含已核銷人數", metric: "checked_in_users", want: "2"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("子測試：%s", tt.progress)
			utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", tt.progress))
			if got := metrics[tt.metric]; got != tt.want {
				errs.Add(tt.progress, "預期 CSV metric %s=%s，實際為 %q", tt.metric, tt.want, got)
			}
		})
	}

	t.Run("核銷率", func(t *testing.T) {
		progress := "CSV 需包含正確核銷率"
		t.Logf("子測試：%s", progress)
		utils.PrintTestProgress(fmt.Sprintf("子測試：%s\n", progress))

		got, ok := metrics["check_in_rate"]
		if !ok {
			errs.Add(progress, "預期 CSV 內含 check_in_rate metric")
			return
		}
		rate, err := strconv.ParseFloat(got, 64)
		if err != nil {
			errs.Add(progress, "預期 check_in_rate 可解析為數字，實際為 %q：%v", got, err)
			return
		}
		if !assertHRFloatEquals(rate, 66.6666666667) {
			errs.Add(progress, "預期 check_in_rate 約為 66.6667，實際為 %.6f", rate)
		}
	})

	utils.PrintTestProgress("==================================================\n\n")
}
