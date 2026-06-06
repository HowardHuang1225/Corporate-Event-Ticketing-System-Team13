package manager

import (
	"fmt"
	"testing"
	"time"

	"ticketing-system/backend/model"
	totputil "ticketing-system/backend/pkg/totp"
	"ticketing-system/backend/repository"
	ticketsvc "ticketing-system/backend/service/ticket"
	utils "ticketing-system/backend/test_utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var managerTicketLifecycleTestModels = []any{
	&model.User{},
	&model.Event{},
	&model.TicketType{},
	&model.Application{},
	&model.Ticket{},
	&model.Checkin{},
}

type managerCheckinResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Message string       `json:"message"`
		Ticket  model.Ticket `json:"ticket"`
	} `json:"data"`
}

type managerTicketLifecycleUsers struct {
	Manager  model.User
	Employee model.User
}

func setupManagerTicketLifecycleTest(t *testing.T) (*gorm.DB, managerTicketLifecycleUsers, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, managerTicketLifecycleTestModels)
	if err != nil {
		return nil, managerTicketLifecycleUsers{}, nil, err
	}

	suffix := utils.UniqueTestSuffix()
	seeded, err := utils.SeedTestRole(tx, []model.User{
		{
			EmployeeID:   fmt.Sprintf("TICKETMGR%s", suffix),
			Name:         "Ticket Test Manager",
			Email:        fmt.Sprintf("ticket-manager-%s@example.com", suffix),
			Department:   "Events",
			Region:       "Tainan",
			Role:         "event_manager",
			PasswordHash: "",
			IsActive:     true,
		},
		{
			EmployeeID:   fmt.Sprintf("TICKETEMP%s", suffix),
			Name:         "Ticket Test Employee",
			Email:        fmt.Sprintf("ticket-employee-%s@example.com", suffix),
			Department:   "Engineering",
			Region:       "Tainan",
			Role:         "employee",
			PasswordHash: "",
			IsActive:     true,
		},
	}, true)
	if err != nil {
		_ = tx.Rollback()
		return nil, managerTicketLifecycleUsers{}, nil, err
	}

	return tx, managerTicketLifecycleUsers{Manager: seeded[0], Employee: seeded[1]}, cleanup, nil
}

func newManagerTicketLifecycleRouter(db *gorm.DB, manager model.User) *gin.Engine {
	repos := repository.New(db, nil)
	ticketService := ticketsvc.New(repos)

	handler := New(nil, ticketService, nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.POST("/checkin", handler.Checkin)
	return router
}

func seedManagerCheckinTicket(db *gorm.DB, users managerTicketLifecycleUsers, isUsed bool) (model.Ticket, error) {
	now := time.Now().UTC().Truncate(time.Second)
	event := model.Event{
		Title:               fmt.Sprintf("核銷測試活動 %s", utils.UniqueTestSuffix()),
		Description:         "管理者核銷測試資料",
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
		return model.Ticket{}, fmt.Errorf("failed to seed check-in event: %w", err)
	}

	ticketType := model.TicketType{
		EventID:    event.ID,
		Name:       "一般票",
		TotalQuota: 10,
		Remaining:  9,
		Version:    1,
		CreatedAt:  now,
	}
	if err := db.Create(&ticketType).Error; err != nil {
		return model.Ticket{}, fmt.Errorf("failed to seed check-in ticket type: %w", err)
	}

	app := model.Application{
		UserID:         users.Employee.ID,
		EventID:        event.ID,
		TicketTypeID:   ticketType.ID,
		Quantity:       1,
		Status:         "approved",
		IdempotencyKey: fmt.Sprintf("checkin-%s", utils.UniqueTestSuffix()),
	}
	if err := db.Create(&app).Error; err != nil {
		return model.Ticket{}, fmt.Errorf("failed to seed check-in application: %w", err)
	}

	ticket := model.Ticket{
		ApplicationID: app.ID,
		UserID:        users.Employee.ID,
		EventID:       event.ID,
		TicketTypeID:  ticketType.ID,
		QRToken:       uuid.New().String(),
		IsUsed:        isUsed,
		ExpiresAt:     event.EndTime,
	}
	if err := db.Create(&ticket).Error; err != nil {
		return model.Ticket{}, fmt.Errorf("failed to seed ticket for check-in test: %w", err)
	}
	return ticket, nil
}

func decodeManagerCheckinResponse(body []byte) (managerCheckinResponse, error) {
	return utils.DecodeJSON[managerCheckinResponse](body)
}

func managerTicketByID(db *gorm.DB, ticketID uuid.UUID) (model.Ticket, error) {
	var ticket model.Ticket
	if err := db.First(&ticket, "id = ?", ticketID).Error; err != nil {
		return model.Ticket{}, fmt.Errorf("failed to query ticket %s: %w", ticketID, err)
	}
	return ticket, nil
}

func managerTicketCheckinCount(db *gorm.DB, ticketID uuid.UUID) (int64, error) {
	var count int64
	if err := db.Model(&model.Checkin{}).Where("ticket_id = ?", ticketID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count check-ins for ticket %s: %w", ticketID, err)
	}
	return count, nil
}

func managerCurrentWindowDynamicQRToken(baseToken string) string {
	return baseToken + "|" + totputil.Generate(baseToken, time.Now().Unix()/60)
}
