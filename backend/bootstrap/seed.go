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
		{EmployeeID: "MGR001", Name: "王大明", Email: "manager@company.com", Department: "福委會", Region: "台南廠", Role: "event_manager", PasswordHash: hash("password")},
		{EmployeeID: "EMP001", Name: "李小花", Email: "emp001@company.com", Department: "工程部", Region: "台南廠", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "EMP002", Name: "張三", Email: "emp002@company.com", Department: "人事部", Region: "新竹廠", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "EMP003", Name: "陳小明", Email: "emp003@company.com", Department: "製程部", Region: "台南廠", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "EMP004", Name: "林美玲", Email: "emp004@company.com", Department: "設計部", Region: "台南廠", Role: "employee", PasswordHash: hash("password")},
		{EmployeeID: "HR001", Name: "陳副理", Email: "hr@company.com", Department: "人資部", Region: "台南廠", Role: "hr", PasswordHash: hash("password")},
	}
	db.Create(&users)

	managerID := users[0].ID
	now := time.Now()
	sp := func(value string) *string { return &value }

	seeds := []struct {
		event model.Event
		types []model.TicketType
	}{
		{
			event: model.Event{
				Title: "2024 藝文展覽 — 當代水墨特展", Venue: "台南市立美術館",
				Description:         "精選 20 位知名藝術家的水墨作品，帶您感受東方藝術的魅力。",
				StartTime:           now.Add(30 * 24 * time.Hour), EndTime: now.Add(31 * 24 * time.Hour),
				ApplyDeadline: now.Add(20 * 24 * time.Hour), Status: "published",
				RegionRestriction: sp("台南"), MaxTicketsPerPerson: 2, CreatedBy: managerID,
			},
			types: []model.TicketType{{Name: "一般票", TotalQuota: 100, Remaining: 100}, {Name: "眷屬票", TotalQuota: 50, Remaining: 50}},
		},
		{
			event: model.Event{
				Title: "員工家庭日 — 六福村主題樂園", Venue: "六福村主題樂園",
				Description:         "一年一度的員工家庭日！費用全額補助。",
				StartTime:           now.Add(45 * 24 * time.Hour), EndTime: now.Add(45*24*time.Hour + 8*time.Hour),
				ApplyDeadline: now.Add(30 * 24 * time.Hour), Status: "published",
				RegionRestriction: nil, MaxTicketsPerPerson: 4, CreatedBy: managerID,
			},
			types: []model.TicketType{{Name: "員工票（含眷屬 3 人）", TotalQuota: 200, Remaining: 200}},
		},
		{
			event: model.Event{
				Title: "AI 技能提升講座", Venue: "台積電研發大樓 B1 大講堂",
				Description:         "業界專家分享 AI 工具應用。",
				StartTime:           now.Add(15 * 24 * time.Hour), EndTime: now.Add(15*24*time.Hour + 4*time.Hour),
				ApplyDeadline: now.Add(7 * 24 * time.Hour), Status: "draft",
				RegionRestriction: nil, MaxTicketsPerPerson: 1, CreatedBy: managerID,
			},
			types: []model.TicketType{{Name: "入場票", TotalQuota: 300, Remaining: 300}},
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
	fmt.Println("   MGR001 / password  → event_manager")
	fmt.Println("   EMP001 / password  → employee (台南廠)")
	fmt.Println("   EMP002 / password  → employee (新竹廠)")
	fmt.Println("   HR001  / password  → hr")
}
