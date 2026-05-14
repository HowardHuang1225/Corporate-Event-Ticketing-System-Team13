package hr

import (
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	reportsvc "ticketing-system/backend/service/report"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var hrReportTestModels = []any{
	&model.User{},
	&model.Event{},
	&model.TicketType{},
	&model.Application{},
	&model.Ticket{},
	&model.Checkin{},
}

type hrReportUsers struct {
	HR        model.User
	Manager   model.User
	SalesOne  model.User
	SalesTwo  model.User
	Engineer  model.User
	Marketing model.User
}

type hrEventStatsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Event            model.Event            `json:"event"`
		AppliedApps      int64                  `json:"applied_apps"`
		AppliedTickets   int64                  `json:"applied_tickets"`
		AppliedUsers     int64                  `json:"applied_users"`
		ApprovedApps     int64                  `json:"approved_apps"`
		ApprovedTickets  int64                  `json:"approved_tickets"`
		CancelledTickets int64                  `json:"cancelled_tickets"`
		ApprovedUsers    int64                  `json:"approved_users"`
		TotalTickets     int64                  `json:"total_tickets"`
		CheckedInTickets int64                  `json:"checked_in_tickets"`
		CheckedInUsers   int64                  `json:"checked_in_users"`
		CheckInRate      float64                `json:"check_in_rate"`
		ByDepartment     []reportsvc.DeptStat   `json:"by_department"`
		ByRegion         []reportsvc.RegionStat `json:"by_region"`
		ByTicketType     []reportsvc.TypeStat   `json:"by_ticket_type"`
	} `json:"data"`
}

type hrOverviewResponse struct {
	Success bool                      `json:"success"`
	Data    []reportsvc.EventOverview `json:"data"`
}

type hrReportFixture struct {
	Event   model.Event
	General model.TicketType
	VIP     model.TicketType
}

func setupHRReportTest(t *testing.T) (*gorm.DB, hrReportUsers, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, hrReportTestModels)
	if err != nil {
		return nil, hrReportUsers{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	seeded, err := utils.SeedTestRole(tx, []model.User{
		{
			EmployeeID: fmt.Sprintf("HR%s", suffix),
			Name:       "HR 報表測試人資",
			Email:      fmt.Sprintf("hr-report-%s@example.com", suffix),
			Department: "人資部",
			Region:     "總部",
			Role:       "hr",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("HRMGR%s", suffix),
			Name:       "HR 報表測試活動管理者",
			Email:      fmt.Sprintf("hr-report-manager-%s@example.com", suffix),
			Department: "活動部",
			Region:     "總部",
			Role:       "event_manager",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("HRSA%s", suffix),
			Name:       "業務同仁一",
			Email:      fmt.Sprintf("hr-sales-one-%s@example.com", suffix),
			Department: "業務部",
			Region:     "北區",
			Role:       "employee",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("HRSB%s", suffix),
			Name:       "業務同仁二",
			Email:      fmt.Sprintf("hr-sales-two-%s@example.com", suffix),
			Department: "業務部",
			Region:     "北區",
			Role:       "employee",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("HRENG%s", suffix),
			Name:       "工程同仁",
			Email:      fmt.Sprintf("hr-engineer-%s@example.com", suffix),
			Department: "工程部",
			Region:     "南區",
			Role:       "employee",
			IsActive:   true,
		},
		{
			EmployeeID: fmt.Sprintf("HRMKT%s", suffix),
			Name:       "行銷同仁",
			Email:      fmt.Sprintf("hr-marketing-%s@example.com", suffix),
			Department: "行銷部",
			Region:     "中區",
			Role:       "employee",
			IsActive:   true,
		},
	}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, hrReportUsers{}, nil, err
	}

	return tx, hrReportUsers{
		HR:        seeded[0],
		Manager:   seeded[1],
		SalesOne:  seeded[2],
		SalesTwo:  seeded[3],
		Engineer:  seeded[4],
		Marketing: seeded[5],
	}, cleanup, nil
}

func newHRReportRouter(db *gorm.DB, user model.User) *gin.Engine {
	repos := repository.New(db, nil)
	handler := New(reportsvc.New(repos))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID.String())
		c.Set("role", "hr")
		c.Next()
	})
	router.GET("/reports/overview", handler.Overview)
	router.GET("/reports/events/:id/stats", handler.EventStats)
	router.GET("/reports/events/:id/export", handler.ExportEventCSV)
	return router
}

func seedHRReportMetricsFixture(db *gorm.DB, users hrReportUsers) (hrReportFixture, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("HR 統計測試活動 %s", utils.UniqueTestSuffix()),
		Description:         "HR 報表統計測試資料",
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
		return hrReportFixture{}, fmt.Errorf("建立 HR 報表活動測試資料失敗: %w", err)
	}

	general := model.TicketType{EventID: event.ID, Name: "一般票", TotalQuota: 10, Remaining: 4}
	vip := model.TicketType{EventID: event.ID, Name: "VIP票", TotalQuota: 5, Remaining: 4}
	for _, ticketType := range []*model.TicketType{&general, &vip} {
		if err := db.Create(ticketType).Error; err != nil {
			return hrReportFixture{}, fmt.Errorf("建立 HR 報表票種測試資料 %q 失敗: %w", ticketType.Name, err)
		}
	}

	aliceApproved, err := seedHRReportApplication(db, users.SalesOne, event, general, "approved", 2, "alice-approved")
	if err != nil {
		return hrReportFixture{}, err
	}
	bobApproved, err := seedHRReportApplication(db, users.SalesTwo, event, vip, "approved", 1, "bob-approved")
	if err != nil {
		return hrReportFixture{}, err
	}
	if _, err := seedHRReportApplication(db, users.Engineer, event, general, "pending", 3, "engineer-pending"); err != nil {
		return hrReportFixture{}, err
	}
	if _, err := seedHRReportApplication(db, users.SalesTwo, event, general, "cancelled", 1, "bob-cancelled"); err != nil {
		return hrReportFixture{}, err
	}

	aliceTickets, err := seedHRReportTickets(db, users.SalesOne, event, general, aliceApproved, 2)
	if err != nil {
		return hrReportFixture{}, err
	}
	bobTickets, err := seedHRReportTickets(db, users.SalesTwo, event, vip, bobApproved, 1)
	if err != nil {
		return hrReportFixture{}, err
	}
	if err := seedHRReportCheckin(db, users.Manager, aliceTickets[0]); err != nil {
		return hrReportFixture{}, err
	}
	if err := seedHRReportCheckin(db, users.Manager, bobTickets[0]); err != nil {
		return hrReportFixture{}, err
	}

	if err := db.Preload("TicketTypes").First(&event, "id = ?", event.ID).Error; err != nil {
		return hrReportFixture{}, fmt.Errorf("重新讀取 HR 報表活動測試資料失敗: %w", err)
	}
	return hrReportFixture{Event: event, General: general, VIP: vip}, nil
}

func seedHRReportPendingOnlyEvent(db *gorm.DB, users hrReportUsers) (model.Event, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("HR 僅待審活動 %s", utils.UniqueTestSuffix()),
		Description:         "HR 總覽零核銷率測試資料",
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
		return model.Event{}, fmt.Errorf("建立 HR 僅待審活動測試資料失敗: %w", err)
	}

	ticketType := model.TicketType{EventID: event.ID, Name: "工作坊票", TotalQuota: 8, Remaining: 8}
	if err := db.Create(&ticketType).Error; err != nil {
		return model.Event{}, fmt.Errorf("建立 HR 僅待審活動票種測試資料失敗: %w", err)
	}
	if _, err := seedHRReportApplication(db, users.Marketing, event, ticketType, "pending", 4, "marketing-pending"); err != nil {
		return model.Event{}, err
	}

	return event, nil
}

func seedHRReportApplication(
	db *gorm.DB,
	user model.User,
	event model.Event,
	ticketType model.TicketType,
	status string,
	quantity int,
	key string,
) (model.Application, error) {
	app := model.Application{
		UserID:         user.ID,
		EventID:        event.ID,
		TicketTypeID:   ticketType.ID,
		Quantity:       quantity,
		Status:         status,
		IdempotencyKey: fmt.Sprintf("hr-report-%s-%s", key, utils.UniqueTestSuffix()),
	}
	if err := db.Create(&app).Error; err != nil {
		return model.Application{}, fmt.Errorf("建立 HR 報表報名測試資料 %q 失敗: %w", key, err)
	}
	return app, nil
}

func seedHRReportTickets(
	db *gorm.DB,
	user model.User,
	event model.Event,
	ticketType model.TicketType,
	app model.Application,
	count int,
) ([]model.Ticket, error) {
	tickets := make([]model.Ticket, 0, count)
	for index := 0; index < count; index++ {
		ticket := model.Ticket{
			ApplicationID: app.ID,
			UserID:        user.ID,
			EventID:       event.ID,
			TicketTypeID:  ticketType.ID,
			QRToken:       uuid.New().String(),
			ExpiresAt:     event.EndTime,
		}
		if err := db.Create(&ticket).Error; err != nil {
			return nil, fmt.Errorf("建立第 %d 張 HR 報表票券測試資料失敗: %w", index+1, err)
		}
		tickets = append(tickets, ticket)
	}
	return tickets, nil
}

func seedHRReportCheckin(db *gorm.DB, checker model.User, ticket model.Ticket) error {
	if err := db.Model(&model.Ticket{}).Where("id = ?", ticket.ID).Update("is_used", true).Error; err != nil {
		return fmt.Errorf("標記 HR 報表票券為已使用失敗: %w", err)
	}
	checkin := model.Checkin{
		TicketID:  ticket.ID,
		CheckedBy: checker.ID,
	}
	if err := db.Create(&checkin).Error; err != nil {
		return fmt.Errorf("建立 HR 報表核銷測試資料失敗: %w", err)
	}
	return nil
}

func decodeHREventStatsResponse(body []byte) (hrEventStatsResponse, error) {
	return utils.DecodeJSON[hrEventStatsResponse](body)
}

func decodeHROverviewResponse(body []byte) (hrOverviewResponse, error) {
	return utils.DecodeJSON[hrOverviewResponse](body)
}

func hrDeptCount(stats []reportsvc.DeptStat, department string) (int64, bool) {
	for _, stat := range stats {
		if stat.Department == department {
			return stat.Count, true
		}
	}
	return 0, false
}

func hrRegionCount(stats []reportsvc.RegionStat, region string) (int64, bool) {
	for _, stat := range stats {
		if stat.Region == region {
			return stat.Count, true
		}
	}
	return 0, false
}

func hrTicketTypeStat(stats []reportsvc.TypeStat, name string) (reportsvc.TypeStat, bool) {
	for _, stat := range stats {
		if stat.TicketTypeName == name {
			return stat, true
		}
	}
	return reportsvc.TypeStat{}, false
}

func hrOverviewForEvent(rows []reportsvc.EventOverview, eventID string) (reportsvc.EventOverview, bool) {
	for _, row := range rows {
		if row.EventID == eventID {
			return row, true
		}
	}
	return reportsvc.EventOverview{}, false
}

func assertHRFloatEquals(got float64, want float64) bool {
	return math.Abs(got-want) < 0.0001
}

func readHRMetricCSV(body string) (map[string]string, error) {
	reader := csv.NewReader(strings.NewReader(body))
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("讀取 CSV 回應失敗: %w", err)
	}
	if len(rows) == 0 || len(rows[0]) != 2 || rows[0][0] != "metric" || rows[0][1] != "value" {
		return nil, fmt.Errorf("CSV 標頭不符合預期: %v", rows)
	}

	metrics := make(map[string]string)
	for _, row := range rows[1:] {
		if len(row) != 2 {
			return nil, fmt.Errorf("CSV 資料列不符合預期: %v", row)
		}
		metrics[row[0]] = row[1]
	}
	return metrics, nil
}

func performHRGet(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}
