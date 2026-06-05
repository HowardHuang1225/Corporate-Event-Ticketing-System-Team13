package report

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	"ticketing-system/backend/service/apperror"
	utils "ticketing-system/backend/test_utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var reportServiceTestModels = []any{
	&model.User{},
	&model.Event{},
	&model.TicketType{},
	&model.Application{},
	&model.Ticket{},
	&model.Checkin{},
}

type reportServiceUsers struct {
	Manager  model.User
	SalesOne model.User
	SalesTwo model.User
	Engineer model.User
}

type reportServiceFixture struct {
	Event   model.Event
	General model.TicketType
	VIP     model.TicketType
}

func TestReportService(t *testing.T) {
	tasks := []utils.Task{
		{
			Description: "測試報表服務可計算單一活動統計與 CSV 匯出",
			Target:      ReportServiceEventStatsAndCSV,
		},
		{
			Description: "測試報表服務可產生活動總覽並處理無票券活動的 0% 核銷率",
			Target:      ReportServiceOverview,
		},
		{
			Description: "測試報表服務查詢不存在活動時回傳 NOT_FOUND",
			Target:      ReportServiceRejectsMissingEvent,
		},
	}

	utils.RunTestTasks(t, tasks)
}

func ReportServiceEventStatsAndCSV(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("報表服務單一活動統計：確認報名、核准、取消、核銷與 CSV 匯出數值。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupReportServiceTest(t)
	if err != nil {
		errs.Add("準備 report service 統計測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	fixture, err := seedReportServiceMetricsFixture(tx, users)
	if err != nil {
		errs.Add("建立 report service 統計測試資料", "%v", err)
		return
	}
	service := New(repository.New(tx, nil))

	stats, err := service.EventStats(fixture.Event.ID.String())
	if err != nil {
		errs.Add("取得單一活動統計", "%v", err)
		return
	}
	if event, ok := stats["event"].(model.Event); !ok || event.ID != fixture.Event.ID {
		errs.Add("檢查活動資料", "預期活動 %s，實際為 %#v", fixture.Event.ID, stats["event"])
		return
	}
	assertReportInt(errs, "applied_apps", stats["applied_apps"], 4)
	assertReportInt(errs, "applied_tickets", stats["applied_tickets"], 7)
	assertReportInt(errs, "applied_users", stats["applied_users"], 3)
	assertReportInt(errs, "approved_apps", stats["approved_apps"], 2)
	assertReportInt(errs, "approved_tickets", stats["approved_tickets"], 3)
	assertReportInt(errs, "approved_users", stats["approved_users"], 2)
	assertReportInt(errs, "cancelled_tickets", stats["cancelled_tickets"], 1)
	assertReportInt(errs, "total_tickets", stats["total_tickets"], 3)
	assertReportInt(errs, "checked_in_tickets", stats["checked_in_tickets"], 2)
	assertReportInt(errs, "checked_in_users", stats["checked_in_users"], 2)
	if rate, ok := stats["check_in_rate"].(float64); !ok || !reportFloatEquals(rate, 66.6666666667) {
		errs.Add("檢查核銷率", "預期約 66.6667，實際為 %#v", stats["check_in_rate"])
		return
	}

	deptStats, ok := stats["by_department"].([]DeptStat)
	if !ok {
		errs.Add("檢查部門統計型別", "實際為 %T", stats["by_department"])
		return
	}
	if count, ok := reportDeptCount(deptStats, "業務部"); !ok || count != 2 {
		errs.Add("檢查部門統計", "預期業務部 2 人，實際 count=%d ok=%v", count, ok)
		return
	}
	typeStats, ok := stats["by_ticket_type"].([]TypeStat)
	if !ok {
		errs.Add("檢查票種統計型別", "實際為 %T", stats["by_ticket_type"])
		return
	}
	general, ok := reportTypeStat(typeStats, "一般票")
	if !ok || general.Total != 6 || general.Approved != 2 || general.Cancelled != 1 || general.Active != 2 {
		errs.Add("檢查一般票統計", "實際為 %+v ok=%v", general, ok)
		return
	}

	body, err := service.ExportEventCSV(fixture.Event.ID.String())
	if err != nil {
		errs.Add("匯出活動 CSV", "%v", err)
		return
	}
	metrics, err := parseReportCSV(body)
	if err != nil {
		errs.Add("解析活動 CSV", "%v", err)
		return
	}
	for key, want := range map[string]string{
		"applied_apps":       "4",
		"applied_tickets":    "7",
		"applied_users":      "3",
		"approved_apps":      "2",
		"approved_tickets":   "3",
		"approved_users":     "2",
		"cancelled_tickets":  "1",
		"total_tickets":      "3",
		"checked_in_tickets": "2",
		"checked_in_users":   "2",
	} {
		if got := metrics[key]; got != want {
			errs.Add("檢查 CSV metric "+key, "預期 %s，實際為 %q", want, got)
			return
		}
	}
	if got, err := strconv.ParseFloat(metrics["check_in_rate"], 64); err != nil || !reportFloatEquals(got, 66.6666666667) {
		errs.Add("檢查 CSV 核銷率", "值=%q err=%v", metrics["check_in_rate"], err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ReportServiceOverview(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("報表服務活動總覽：確認每個活動列的報名與核銷統計。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, users, cleanup, err := setupReportServiceTest(t)
	if err != nil {
		errs.Add("準備 report service 總覽測試資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	fixture, err := seedReportServiceMetricsFixture(tx, users)
	if err != nil {
		errs.Add("建立 report service 總覽統計活動", "%v", err)
		return
	}
	pendingOnly, err := seedReportServicePendingOnlyEvent(tx, users)
	if err != nil {
		errs.Add("建立 report service 無票券活動", "%v", err)
		return
	}

	service := New(repository.New(tx, nil))
	overview, err := service.Overview()
	if err != nil {
		errs.Add("取得活動總覽", "%v", err)
		return
	}

	row, ok := reportOverviewForEvent(overview, fixture.Event.ID.String())
	if !ok {
		errs.Add("查找有核銷活動總覽列", "預期包含 %s", fixture.Event.ID)
		return
	}
	if row.AppliedApps != 4 || row.AppliedTickets != 7 || row.AppliedUsers != 3 ||
		row.ApprovedTickets != 3 || row.ApprovedUsers != 2 || row.Cancelled != 1 ||
		row.TotalTickets != 3 || row.CheckedIn != 2 || !reportFloatEquals(row.CheckInRate, 66.6666666667) {
		errs.Add("檢查有核銷活動總覽列", "row=%+v", row)
		return
	}

	emptyRow, ok := reportOverviewForEvent(overview, pendingOnly.ID.String())
	if !ok {
		errs.Add("查找無票券活動總覽列", "預期包含 %s", pendingOnly.ID)
		return
	}
	if emptyRow.AppliedApps != 1 || emptyRow.AppliedTickets != 4 || emptyRow.AppliedUsers != 1 ||
		emptyRow.ApprovedTickets != 0 || emptyRow.ApprovedUsers != 0 || emptyRow.Cancelled != 0 ||
		emptyRow.TotalTickets != 0 || emptyRow.CheckedIn != 0 || emptyRow.CheckInRate != 0 {
		errs.Add("檢查無票券活動總覽列", "row=%+v", emptyRow)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func ReportServiceRejectsMissingEvent(t *testing.T, errs *utils.Errors) {
	t.Helper()

	utils.PrintTestProgress("報表服務錯誤情境：查詢不存在活動統計或匯出 CSV 時回傳 NOT_FOUND。\n")
	utils.PrintTestProgress("==================================================\n")

	tx, _, cleanup, err := setupReportServiceTest(t)
	if err != nil {
		errs.Add("準備 report service 錯誤情境資料", "%v", err)
		return
	}
	t.Cleanup(cleanup)

	service := New(repository.New(tx, nil))
	missingID := uuid.New().String()
	if _, err := service.EventStats(missingID); assertReportAppError(err, http.StatusNotFound, "NOT_FOUND") != nil {
		errs.Add("查詢不存在活動統計", "實際錯誤 %v", err)
		return
	}
	if _, err := service.ExportEventCSV(missingID); assertReportAppError(err, http.StatusNotFound, "NOT_FOUND") != nil {
		errs.Add("匯出不存在活動 CSV", "實際錯誤 %v", err)
		return
	}

	utils.PrintTestProgress("==================================================\n\n")
}

func setupReportServiceTest(t *testing.T) (*gorm.DB, reportServiceUsers, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, reportServiceTestModels)
	if err != nil {
		return nil, reportServiceUsers{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	seeded, err := utils.SeedTestRole(tx, []model.User{
		{
			EmployeeID: fmt.Sprintf("RPTMGR%s", suffix),
			Name:       "報表測試管理者",
			Email:      fmt.Sprintf("report-manager-%s@example.com", suffix),
			Department: "活動部",
			Region:     "總部",
			Role:       "event_manager",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("RPTSA%s", suffix),
			Name:       "業務同仁一",
			Email:      fmt.Sprintf("report-sales-one-%s@example.com", suffix),
			Department: "業務部",
			Region:     "北區",
			Role:       "employee",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("RPTSB%s", suffix),
			Name:       "業務同仁二",
			Email:      fmt.Sprintf("report-sales-two-%s@example.com", suffix),
			Department: "業務部",
			Region:     "北區",
			Role:       "employee",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("RPTENG%s", suffix),
			Name:       "工程同仁",
			Email:      fmt.Sprintf("report-engineer-%s@example.com", suffix),
			Department: "工程部",
			Region:     "南區",
			Role:       "employee",
			IsActive:   true,
		},
	}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, reportServiceUsers{}, nil, err
	}

	return tx, reportServiceUsers{
		Manager:  seeded[0],
		SalesOne: seeded[1],
		SalesTwo: seeded[2],
		Engineer: seeded[3],
	}, cleanup, nil
}

func seedReportServiceMetricsFixture(db *gorm.DB, users reportServiceUsers) (reportServiceFixture, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("報表服務統計活動 %s", utils.UniqueTestSuffix()),
		Description:         "報表服務統計測試資料",
		Venue:               "總部大禮堂",
		PublishTime:         now.Add(-48 * time.Hour),
		StartTime:           now.Add(72 * time.Hour),
		ApplyDeadline:       now.Add(24 * time.Hour),
		EndTime:             now.Add(76 * time.Hour),
		Status:              "published",
		MaxTicketsPerPerson: 3,
		CreatedBy:           users.Manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return reportServiceFixture{}, fmt.Errorf("建立報表服務活動失敗：%w", err)
	}

	general := model.TicketType{EventID: event.ID, Name: "一般票", TotalQuota: 10, Remaining: 4}
	vip := model.TicketType{EventID: event.ID, Name: "VIP票", TotalQuota: 5, Remaining: 4}
	for _, ticketType := range []*model.TicketType{&general, &vip} {
		if err := db.Create(ticketType).Error; err != nil {
			return reportServiceFixture{}, fmt.Errorf("建立報表服務票種 %q 失敗：%w", ticketType.Name, err)
		}
	}

	aliceApproved, err := seedReportServiceApplication(db, users.SalesOne, event, general, "approved", 2, "alice-approved")
	if err != nil {
		return reportServiceFixture{}, err
	}
	bobApproved, err := seedReportServiceApplication(db, users.SalesTwo, event, vip, "approved", 1, "bob-approved")
	if err != nil {
		return reportServiceFixture{}, err
	}
	if _, err := seedReportServiceApplication(db, users.Engineer, event, general, "pending", 3, "engineer-pending"); err != nil {
		return reportServiceFixture{}, err
	}
	if _, err := seedReportServiceApplication(db, users.SalesTwo, event, general, "cancelled", 1, "bob-cancelled"); err != nil {
		return reportServiceFixture{}, err
	}

	aliceTickets, err := seedReportServiceTickets(db, users.SalesOne, event, general, aliceApproved, 2)
	if err != nil {
		return reportServiceFixture{}, err
	}
	bobTickets, err := seedReportServiceTickets(db, users.SalesTwo, event, vip, bobApproved, 1)
	if err != nil {
		return reportServiceFixture{}, err
	}
	if err := seedReportServiceCheckin(db, users.Manager, aliceTickets[0]); err != nil {
		return reportServiceFixture{}, err
	}
	if err := seedReportServiceCheckin(db, users.Manager, bobTickets[0]); err != nil {
		return reportServiceFixture{}, err
	}

	return reportServiceFixture{Event: event, General: general, VIP: vip}, nil
}

func seedReportServicePendingOnlyEvent(db *gorm.DB, users reportServiceUsers) (model.Event, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("報表服務待審活動 %s", utils.UniqueTestSuffix()),
		Description:         "報表服務零核銷率測試資料",
		Venue:               "分公司會議室",
		PublishTime:         now.Add(-24 * time.Hour),
		StartTime:           now.Add(96 * time.Hour),
		ApplyDeadline:       now.Add(48 * time.Hour),
		EndTime:             now.Add(100 * time.Hour),
		Status:              "published",
		MaxTicketsPerPerson: 4,
		CreatedBy:           users.Manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, fmt.Errorf("建立報表服務待審活動失敗：%w", err)
	}
	ticketType := model.TicketType{EventID: event.ID, Name: "工作坊票", TotalQuota: 8, Remaining: 8}
	if err := db.Create(&ticketType).Error; err != nil {
		return model.Event{}, fmt.Errorf("建立報表服務待審票種失敗：%w", err)
	}
	if _, err := seedReportServiceApplication(db, users.Engineer, event, ticketType, "pending", 4, "engineer-pending-only"); err != nil {
		return model.Event{}, err
	}
	return event, nil
}

func seedReportServiceApplication(db *gorm.DB, user model.User, event model.Event, ticketType model.TicketType, status string, quantity int, suffix string) (model.Application, error) {
	app := model.Application{
		UserID:         user.ID,
		EventID:        event.ID,
		TicketTypeID:   ticketType.ID,
		Quantity:       quantity,
		Status:         status,
		IdempotencyKey: fmt.Sprintf("report-service-%s-%s", suffix, utils.UniqueTestSuffix()),
	}
	if err := db.Create(&app).Error; err != nil {
		return model.Application{}, fmt.Errorf("建立報表服務申請 %s 失敗：%w", suffix, err)
	}
	return app, nil
}

func seedReportServiceTickets(db *gorm.DB, user model.User, event model.Event, ticketType model.TicketType, app model.Application, count int) ([]model.Ticket, error) {
	tickets := make([]model.Ticket, 0, count)
	for i := 0; i < count; i++ {
		ticket := model.Ticket{
			ApplicationID: app.ID,
			UserID:        user.ID,
			EventID:       event.ID,
			TicketTypeID:  ticketType.ID,
			QRToken:       uuid.New().String(),
			ExpiresAt:     event.EndTime,
		}
		if err := db.Create(&ticket).Error; err != nil {
			return nil, fmt.Errorf("建立報表服務票券失敗：%w", err)
		}
		tickets = append(tickets, ticket)
	}
	return tickets, nil
}

func seedReportServiceCheckin(db *gorm.DB, manager model.User, ticket model.Ticket) error {
	checkin := model.Checkin{
		TicketID:  ticket.ID,
		CheckedBy: manager.ID,
	}
	if err := db.Create(&checkin).Error; err != nil {
		return fmt.Errorf("建立報表服務核銷紀錄失敗：%w", err)
	}
	return nil
}

func assertReportInt(errs *utils.Errors, key string, got any, want int64) {
	if got != want {
		errs.Add("檢查 "+key, "預期 %d，實際為 %#v", want, got)
	}
}

func reportDeptCount(stats []DeptStat, department string) (int64, bool) {
	for _, stat := range stats {
		if stat.Department == department {
			return stat.Count, true
		}
	}
	return 0, false
}

func reportTypeStat(stats []TypeStat, name string) (TypeStat, bool) {
	for _, stat := range stats {
		if stat.TicketTypeName == name {
			return stat, true
		}
	}
	return TypeStat{}, false
}

func reportOverviewForEvent(rows []EventOverview, eventID string) (EventOverview, bool) {
	for _, row := range rows {
		if row.EventID == eventID {
			return row, true
		}
	}
	return EventOverview{}, false
}

func parseReportCSV(body string) (map[string]string, error) {
	reader := csv.NewReader(strings.NewReader(body))
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows[0]) != 2 || rows[0][0] != "metric" || rows[0][1] != "value" {
		return nil, fmt.Errorf("unexpected CSV header: %v", rows)
	}
	metrics := map[string]string{}
	for _, row := range rows[1:] {
		if len(row) != 2 {
			return nil, fmt.Errorf("unexpected CSV row: %v", row)
		}
		metrics[row[0]] = row[1]
	}
	return metrics, nil
}

func reportFloatEquals(got, want float64) bool {
	if got > want {
		return got-want < 0.0001
	}
	return want-got < 0.0001
}

func assertReportAppError(err error, wantStatus int, wantCode string) error {
	if err == nil {
		return fmt.Errorf("預期錯誤代碼 %q，實際沒有錯誤", wantCode)
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		return fmt.Errorf("預期 apperror.Error，實際錯誤為 %T：%v", err, err)
	}
	if appErr.Status != wantStatus {
		return fmt.Errorf("預期 HTTP 狀態 %d，實際為 %d", wantStatus, appErr.Status)
	}
	if appErr.Code != wantCode {
		return fmt.Errorf("預期錯誤代碼 %q，實際為 %q", wantCode, appErr.Code)
	}
	return nil
}
