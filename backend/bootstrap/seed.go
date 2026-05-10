package bootstrap

import (
	"fmt"
	"log"
	"time"

	"ticketing-system/backend/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func SeedDemoData(db *gorm.DB) {
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count > 0 {
		log.Println("Seed data already exists")
		return
	}
	log.Println("Seeding demo data...")

	hash := func(password string) string {
		value, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		return string(value)
	}

	users := []model.User{
		{EmployeeID: "MGR001", Name: "Event Manager", Email: "manager@company.com", Department: "Welfare Committee", Region: "Tainan", Role: "event_manager", PasswordHash: hash("password")},
		{EmployeeID: "EMP001", Name: "Tainan Employee", Email: "emp001@company.com", Department: "Engineering", Region: "Tainan", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "EMP002", Name: "Hsinchu Employee", Email: "emp002@company.com", Department: "Engineering", Region: "Hsinchu", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "EMP003", Name: "Operations Employee", Email: "emp003@company.com", Department: "Operations", Region: "Tainan", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "EMP004", Name: "Sales Employee", Email: "emp004@company.com", Department: "Sales", Region: "Tainan", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "HR001", Name: "HR Analyst", Email: "hr@company.com", Department: "Human Resources", Region: "Tainan", Role: "hr", PasswordHash: hash("password")},
	}
	db.Create(&users)

	managerID := users[0].ID
	now := time.Now()
	stringPtr := func(value string) *string { return &value }

	seeds := []struct {
		event model.Event
		types []model.TicketType
	}{
		{
			event: model.Event{
				Title:               "Company Family Day",
				Venue:               "Tainan Main Hall",
				Description:         "Annual company family day event.",
				PublishTime:         now.Add(10 * 24 * time.Hour),
				StartTime:           now.Add(30 * 24 * time.Hour),
				EndTime:             now.Add(31 * 24 * time.Hour),
				ApplyDeadline:       now.Add(30*24*time.Hour + 12*time.Hour),
				Status:              "published",
				RegionRestriction:   stringPtr("Tainan"),
				MaxTicketsPerPerson: 2,
				CreatedBy:           managerID,
			},
			types: []model.TicketType{
				{Name: "General", TotalQuota: 100, Remaining: 100},
				{Name: "Family", TotalQuota: 50, Remaining: 50},
			},
		},
		{
			event: model.Event{
				Title:               "Wellness Workshop",
				Venue:               "Online",
				Description:         "A wellness workshop open to all employees.",
				PublishTime:         now.Add(25 * 24 * time.Hour),
				StartTime:           now.Add(45 * 24 * time.Hour),
				EndTime:             now.Add(45*24*time.Hour + 8*time.Hour),
				ApplyDeadline:       now.Add(45*24*time.Hour + 4*time.Hour),
				Status:              "published",
				RegionRestriction:   nil,
				MaxTicketsPerPerson: 4,
				CreatedBy:           managerID,
			},
			types: []model.TicketType{
				{Name: "Workshop Seat", TotalQuota: 200, Remaining: 200},
			},
		},
		{
			event: model.Event{
				Title:               "AI Tech Talk",
				Venue:               "Hsinchu Auditorium B1",
				Description:         "Internal AI sharing session.",
				PublishTime:         now.Add(3 * 24 * time.Hour),
				StartTime:           now.Add(15 * 24 * time.Hour),
				EndTime:             now.Add(15*24*time.Hour + 4*time.Hour),
				ApplyDeadline:       now.Add(15*24*time.Hour + 2*time.Hour),
				Status:              "draft",
				RegionRestriction:   nil,
				MaxTicketsPerPerson: 1,
				CreatedBy:           managerID,
			},
			types: []model.TicketType{
				{Name: "Standard", TotalQuota: 300, Remaining: 300},
			},
		},
	}

	for _, seed := range seeds {
		db.Create(&seed.event)
		for i := range seed.types {
			seed.types[i].EventID = seed.event.ID
			db.Create(&seed.types[i])
		}
	}

	fmt.Println("Seed complete!")
	fmt.Println("   MGR001 / password  -> event_manager")
	fmt.Println("   EMP001 / password  -> employee (Tainan)")
	fmt.Println("   EMP002 / password  -> employee (Hsinchu)")
	fmt.Println("   HR001  / password  -> hr")
}
