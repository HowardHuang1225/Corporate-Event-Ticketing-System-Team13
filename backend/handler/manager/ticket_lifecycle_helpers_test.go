package manager

import (
	"fmt"
	"testing"

	"ticketing-system/backend/model"
	"ticketing-system/backend/repository"
	eventsvc "ticketing-system/backend/service/event"
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

func setupManagerTicketLifecycleTest(t *testing.T) (*gorm.DB, managerApplicationUsers, func(), error) {
	t.Helper()

	tx, cleanup, err := utils.BeginTestTransaction(t, managerTicketLifecycleTestModels)
	if err != nil {
		return nil, managerApplicationUsers{}, nil, err
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
		return nil, managerApplicationUsers{}, nil, err
	}

	return tx, managerApplicationUsers{Manager: seeded[0], Employee: seeded[1]}, cleanup, nil
}

func newManagerTicketLifecycleRouter(db *gorm.DB, manager model.User) *gin.Engine {
	repos := repository.New(db, nil)
	eventService := eventsvc.New(repos)
	ticketService := ticketsvc.New(repos)

	handler := New(eventService, ticketService, nil, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", manager.ID.String())
		c.Set("role", "event_manager")
		c.Next()
	})
	router.POST("/applications/:id/approve", handler.ApproveApplication)
	router.POST("/checkin", handler.Checkin)
	return router
}

func seedManagerCheckinTicket(db *gorm.DB, users managerApplicationUsers, isUsed bool) (model.Ticket, error) {
	event, ticketType, app, err := seedManagerApplicationFixture(db, users, "approved", 1, 9)
	if err != nil {
		return model.Ticket{}, err
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

func managerPersistedTicketsForApplication(db *gorm.DB, appID uuid.UUID) ([]model.Ticket, error) {
	var tickets []model.Ticket
	if err := db.Where("application_id = ?", appID).Order("issued_at asc").Find(&tickets).Error; err != nil {
		return nil, fmt.Errorf("failed to query generated tickets: %w", err)
	}
	return tickets, nil
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
