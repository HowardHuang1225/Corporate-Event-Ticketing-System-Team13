package manager

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	ticketsvc "ticketing-system/backend/service/ticket"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var managerApplicationTestModels = []any{
	&model.User{},
	&model.Event{},
	&model.TicketType{},
	&model.Application{},
	&model.Ticket{},
}

type managerApplicationUsers struct {
	Manager  model.User
	Employee model.User
}

type managerApplicationResponse struct {
	Success bool              `json:"success"`
	Data    model.Application `json:"data"`
}

type managerApplicationListResponse struct {
	Success bool                `json:"success"`
	Data    []model.Application `json:"data"`
}

func setupManagerApplicationTest(t *testing.T) (*gorm.DB, managerApplicationUsers, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, managerApplicationTestModels)
	if err != nil {
		return nil, managerApplicationUsers{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	seeded, err := utils.SeedTestRole(tx, []model.User{
		{
			EmployeeID:   fmt.Sprintf("APPMGR%s", suffix),
			Name:         "審核測試管理者",
			Email:        fmt.Sprintf("application-manager-%s@example.com", suffix),
			Department:   "Events",
			Region:       "Tainan",
			Role:         "event_manager",
			PasswordHash: "",
			IsActive:     true,
		},
		{
			EmployeeID:   fmt.Sprintf("APPEMP%s", suffix),
			Name:         "審核測試員工",
			Email:        fmt.Sprintf("application-employee-%s@example.com", suffix),
			Department:   "Engineering",
			Region:       "Tainan",
			Role:         "employee",
			PasswordHash: "",
			IsActive:     true,
		},
	}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, managerApplicationUsers{}, nil, err
	}

	return tx, managerApplicationUsers{Manager: seeded[0], Employee: seeded[1]}, cleanup, nil
}

func newManagerApplicationRouter(db *gorm.DB, manager model.User) *gin.Engine {
	repos := repository.New(db, nil)
	handler := New(nil, ticketsvc.New(repos), nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.POST("/applications/:id/approve", handler.ApproveApplication)
	router.POST("/applications/:id/reject", handler.RejectApplication)
	return router
}

func newManagerBatchApplicationRouter(db *gorm.DB, manager model.User) *gin.Engine {
	repos := repository.New(db, nil)
	handler := New(nil, ticketsvc.New(repos), nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.POST("/applications/batch/approve", managerHandlerMethod(handler, "BatchApproveApplications"))
	router.POST("/applications/batch/reject", managerHandlerMethod(handler, "BatchRejectApplications"))
	return router
}

func managerHandlerMethod(handler *Handler, methodName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := reflect.ValueOf(handler).MethodByName(methodName)
		contextType := reflect.TypeOf((*gin.Context)(nil))
		if !method.IsValid() || method.Type().NumIn() != 1 || !contextType.AssignableTo(method.Type().In(0)) {
			c.JSON(http.StatusNotImplemented, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "NOT_IMPLEMENTED",
					"message": methodName + " is not implemented",
				},
			})
			return
		}
		method.Call([]reflect.Value{reflect.ValueOf(c)})
	}
}

func seedManagerApplicationFixture(
	db *gorm.DB,
	users managerApplicationUsers,
	status string,
	quantity int,
	remaining int,
) (model.Event, model.TicketType, model.Application, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("審核申請測試活動 %s", utils.UniqueTestSuffix()),
		Description:         "管理者審核申請處理器測試資料",
		Venue:               "主會場",
		PublishTime:         now.Add(-time.Hour),
		StartTime:           now.Add(24 * time.Hour),
		ApplyDeadline:       now.Add(12 * time.Hour),
		EndTime:             now.Add(26 * time.Hour),
		Status:              "published",
		MaxTicketsPerPerson: 3,
		CreatedBy:           users.Manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, model.TicketType{}, model.Application{}, fmt.Errorf("建立審核申請活動測試資料失敗: %w", err)
	}

	ticketType := model.TicketType{
		EventID:    event.ID,
		Name:       "一般票",
		TotalQuota: 20,
		Remaining:  remaining,
		Version:    1,
		CreatedAt:  now,
	}
	if err := db.Create(&ticketType).Error; err != nil {
		return model.Event{}, model.TicketType{}, model.Application{}, fmt.Errorf("建立審核申請票種測試資料失敗: %w", err)
	}

	app := model.Application{
		UserID:         users.Employee.ID,
		EventID:        event.ID,
		TicketTypeID:   ticketType.ID,
		Quantity:       quantity,
		Status:         status,
		IdempotencyKey: fmt.Sprintf("review-%s", utils.UniqueTestSuffix()),
	}
	if err := db.Create(&app).Error; err != nil {
		return model.Event{}, model.TicketType{}, model.Application{}, fmt.Errorf("建立待審核申請測試資料失敗: %w", err)
	}

	return event, ticketType, app, nil
}

func seedManagerBatchApplicationsFixture(
	db *gorm.DB,
	users managerApplicationUsers,
	statuses []string,
	quantities []int,
	remaining int,
) (model.Event, model.TicketType, []model.Application, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("批次審核申請測試活動 %s", utils.UniqueTestSuffix()),
		Description:         "管理者批次審核申請處理器測試資料",
		Venue:               "主會場",
		PublishTime:         now.Add(-time.Hour),
		StartTime:           now.Add(24 * time.Hour),
		ApplyDeadline:       now.Add(12 * time.Hour),
		EndTime:             now.Add(26 * time.Hour),
		Status:              "published",
		MaxTicketsPerPerson: 5,
		CreatedBy:           users.Manager.ID,
	}
	if err := db.Create(&event).Error; err != nil {
		return model.Event{}, model.TicketType{}, nil, fmt.Errorf("建立批次審核活動測試資料失敗: %w", err)
	}

	ticketType := model.TicketType{
		EventID:    event.ID,
		Name:       "一般票",
		TotalQuota: 20,
		Remaining:  remaining,
		Version:    1,
	}
	if err := db.Create(&ticketType).Error; err != nil {
		return model.Event{}, model.TicketType{}, nil, fmt.Errorf("建立批次審核票種測試資料失敗: %w", err)
	}

	apps := make([]model.Application, 0, len(statuses))
	for index, status := range statuses {
		quantity := 1
		if index < len(quantities) {
			quantity = quantities[index]
		}
		app := model.Application{
			UserID:         users.Employee.ID,
			EventID:        event.ID,
			TicketTypeID:   ticketType.ID,
			Quantity:       quantity,
			Status:         status,
			IdempotencyKey: fmt.Sprintf("batch-review-%d-%s", index, utils.UniqueTestSuffix()),
		}
		if err := db.Create(&app).Error; err != nil {
			return model.Event{}, model.TicketType{}, nil, fmt.Errorf("建立第 %d 筆批次審核申請測試資料失敗: %w", index+1, err)
		}
		apps = append(apps, app)
	}

	return event, ticketType, apps, nil
}

func decodeManagerApplicationResponse(body []byte) (managerApplicationResponse, error) {
	return utils.DecodeJSON[managerApplicationResponse](body)
}

func decodeManagerApplicationListResponse(body []byte) (managerApplicationListResponse, error) {
	return utils.DecodeJSON[managerApplicationListResponse](body)
}

func managerReviewTicketCount(db *gorm.DB, appID string) (int64, error) {
	var count int64
	if err := db.Model(&model.Ticket{}).Where("application_id = ?", appID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("計算審核後票券數量失敗: %w", err)
	}
	return count, nil
}

func managerReviewTicketTypeRemaining(db *gorm.DB, ticketTypeID string) (int, error) {
	var remaining int
	if err := db.Model(&model.TicketType{}).
		Select("remaining").
		Where("id = ?", ticketTypeID).
		Scan(&remaining).Error; err != nil {
		return 0, fmt.Errorf("查詢審核後票種庫存失敗: %w", err)
	}
	return remaining, nil
}
