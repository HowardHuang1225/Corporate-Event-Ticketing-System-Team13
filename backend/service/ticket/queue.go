package ticket

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"ticketing-system/backend/model"
	"ticketing-system/backend/pkg"
	"ticketing-system/backend/service/apperror"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type QueueOptions struct {
	Enabled            bool
	Stream             string
	Group              string
	ConsumerPrefix     string
	Workers            int
	ReservationTTL     time.Duration
	MaxWaiting         int64
	ReadBlock          time.Duration
	ReadCount          int64
	MaxProcessAttempts int
	StatusTTL          time.Duration
}

type QueueStatus struct {
	Status         string `json:"status"`
	QueueID        string `json:"queue_id,omitempty"`
	ApplicationID  string `json:"application_id,omitempty"`
	EventID        string `json:"event_id,omitempty"`
	TicketTypeID   string `json:"ticket_type_id,omitempty"`
	Quantity       string `json:"quantity,omitempty"`
	Reason         string `json:"reason,omitempty"`
	QueuedAt       string `json:"queued_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

func loadQueueOptionsFromEnv() QueueOptions {
	return QueueOptions{
		Enabled:            getEnvBool("TICKET_QUEUE_ENABLED", false),
		Stream:             getEnvString("TICKET_QUEUE_STREAM", "ticket:applications"),
		Group:              getEnvString("TICKET_QUEUE_GROUP", "ticket-workers"),
		ConsumerPrefix:     getEnvString("TICKET_QUEUE_CONSUMER_PREFIX", "worker"),
		Workers:            getEnvInt("TICKET_QUEUE_WORKERS", 8),
		ReservationTTL:     time.Duration(getEnvInt("TICKET_QUEUE_RESERVATION_TTL_SECONDS", 900)) * time.Second,
		MaxWaiting:         int64(getEnvInt("TICKET_QUEUE_MAX_WAITING", 100000)),
		ReadBlock:          time.Duration(getEnvInt("TICKET_QUEUE_READ_BLOCK_SECONDS", 5)) * time.Second,
		ReadCount:          int64(getEnvInt("TICKET_QUEUE_READ_COUNT", 10)),
		MaxProcessAttempts: getEnvInt("TICKET_QUEUE_PROCESS_ATTEMPTS", 3),
		StatusTTL:          time.Duration(getEnvInt("TICKET_QUEUE_STATUS_TTL_SECONDS", 3600)) * time.Second,
	}
}

func getEnvString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func (s *Service) ApplyQueued(userID uuid.UUID, req ApplyRequest) (ApplyResult, error) {
	eventID, err := uuid.Parse(req.EventID)
	if err != nil {
		return ApplyResult{}, apperror.Validation("Invalid event_id")
	}
	ticketTypeID, err := uuid.Parse(req.TicketTypeID)
	if err != nil {
		return ApplyResult{}, apperror.Validation("Invalid ticket_type_id")
	}
	if s.redis == nil {
		return ApplyResult{}, apperror.Busy("Queue system busy")
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return ApplyResult{}, apperror.Validation("idempotency_key is required")
	}

	var existing model.Application
	if result := s.db.Where("idempotency_key = ? AND user_id = ?", req.IdempotencyKey, userID.String()).Limit(1).Find(&existing); result.Error != nil {
		return ApplyResult{}, result.Error
	} else if result.RowsAffected > 0 {
		return ApplyResult{Application: existing, Created: false}, nil
	}

	ctx := context.Background()
	statusKey := queueStatusKey(userID.String(), req.IdempotencyKey)
	reservationKey := queueReservationKey(userID.String(), req.IdempotencyKey)
	if fields, err := s.redis.HGetAll(ctx, statusKey).Result(); err == nil && len(fields) > 0 {
		queuedApp := queuedApplicationFromStatus(userID, eventID, ticketTypeID, req, fields)
		return ApplyResult{Application: queuedApp, Created: false, Queued: true, QueueID: fields["queue_id"]}, nil
	}

	if s.queue.MaxWaiting > 0 {
		waiting, err := s.redis.XLen(ctx, s.queue.Stream).Result()
		if err == nil && waiting >= s.queue.MaxWaiting {
			return ApplyResult{}, apperror.New(429, "WAITING_ROOM_FULL", "Too many users are currently waiting. Please try again later.")
		}
	}

	var event model.Event
	if err := s.db.First(&event, "id = ?", eventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ApplyResult{}, apperror.NotFound("Event not found")
		}
		return ApplyResult{}, apperror.Busy("Database system busy")
	}
	if event.Status != "published" {
		return ApplyResult{}, apperror.New(400, "EVENT_NOT_AVAILABLE", "Event is not accepting applications")
	}
	if time.Now().After(event.ApplyDeadline) {
		return ApplyResult{}, apperror.New(400, "APPLY_DEADLINE_PASSED", "Application deadline has passed")
	}

	var issuedCount int64
	if err := s.db.Model(&model.Ticket{}).Where("user_id = ? AND event_id = ?", userID.String(), eventID.String()).Count(&issuedCount).Error; err != nil {
		return ApplyResult{}, apperror.Busy("Database system busy")
	}
	var pendingCount int
	if err := s.db.Model(&model.Application{}).
		Where("user_id = ? AND event_id = ? AND status IN ?", userID.String(), eventID.String(), []string{"pending", "queued", "processing"}).
		Select("COALESCE(SUM(quantity), 0)").
		Scan(&pendingCount).Error; err != nil {
		return ApplyResult{}, apperror.Busy("Database system busy")
	}
	remainingAllowance := event.MaxTicketsPerPerson - int(issuedCount) - pendingCount
	if req.Quantity > remainingAllowance {
		return ApplyResult{}, apperror.New(400, "EXCEEDS_MAX_TICKETS", fmt.Sprintf("You have already applied for %d tickets. The limit is %d. You can only apply for %d more.", int(issuedCount)+pendingCount, event.MaxTicketsPerPerson, remainingAllowance))
	}

	if err := s.ensureInventoryLoaded(ctx, ticketTypeID, req.TicketTypeID); err != nil {
		return ApplyResult{}, err
	}

	queueID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	inventoryKey := "inventory:" + req.TicketTypeID
	userEventReservationKey := queueUserEventReservationKey(userID.String(), req.EventID)
	status, err := s.redis.Eval(ctx, queueReservationLua, []string{
		inventoryKey,
		reservationKey,
		s.queue.Stream,
		statusKey,
		userEventReservationKey,
	},
		req.Quantity,
		int(s.queue.ReservationTTL.Seconds()),
		queueID,
		userID.String(),
		req.EventID,
		req.TicketTypeID,
		req.IdempotencyKey,
		now,
		remainingAllowance,
	).Text()
	if err != nil {
		return ApplyResult{}, apperror.New(503, "QUEUE_BUSY", "Queue system busy")
	}
	switch status {
	case "QUEUED":
		app := model.Application{
			ID:             uuid.MustParse(queueID),
			UserID:         userID,
			EventID:        eventID,
			TicketTypeID:   ticketTypeID,
			Quantity:       req.Quantity,
			Status:         "queued",
			IdempotencyKey: req.IdempotencyKey,
		}
		return ApplyResult{Application: app, Created: true, Queued: true, QueueID: queueID}, nil
	case "DUPLICATE":
		fields, _ := s.redis.HGetAll(ctx, statusKey).Result()
		app := queuedApplicationFromStatus(userID, eventID, ticketTypeID, req, fields)
		return ApplyResult{Application: app, Created: false, Queued: true, QueueID: fields["queue_id"]}, nil
	case "SOLD_OUT":
		return ApplyResult{}, apperror.Conflict("TICKET_SOLD_OUT", "Not enough tickets remaining")
	case "EXCEEDS_MAX_TICKETS":
		return ApplyResult{}, apperror.New(400, "EXCEEDS_MAX_TICKETS", "Ticket limit exceeded")
	default:
		return ApplyResult{}, apperror.New(503, "QUEUE_BUSY", "Queue system busy")
	}
}

func (s *Service) QueueStatus(userID uuid.UUID, idempotencyKey string) (QueueStatus, error) {
	var existing model.Application
	if err := s.db.Where("idempotency_key = ? AND user_id = ?", idempotencyKey, userID.String()).First(&existing).Error; err == nil {
		return QueueStatus{
			Status:         existing.Status,
			ApplicationID:  existing.ID.String(),
			EventID:        existing.EventID.String(),
			TicketTypeID:   existing.TicketTypeID.String(),
			Quantity:       strconv.Itoa(existing.Quantity),
			IdempotencyKey: existing.IdempotencyKey,
		}, nil
	}
	if s.redis == nil {
		return QueueStatus{}, apperror.NotFound("Queue status not found")
	}
	fields, err := s.redis.HGetAll(context.Background(), queueStatusKey(userID.String(), idempotencyKey)).Result()
	if err != nil || len(fields) == 0 {
		return QueueStatus{}, apperror.NotFound("Queue status not found")
	}
	return QueueStatus{
		Status:         fields["status"],
		QueueID:        fields["queue_id"],
		ApplicationID:  fields["application_id"],
		EventID:        fields["event_id"],
		TicketTypeID:   fields["ticket_type_id"],
		Quantity:       fields["quantity"],
		Reason:         fields["reason"],
		QueuedAt:       fields["queued_at"],
		UpdatedAt:      fields["updated_at"],
		IdempotencyKey: idempotencyKey,
	}, nil
}

func (s *Service) StartQueueWorkers(ctx context.Context) {
	if !s.queue.Enabled || s.redis == nil || s.queue.Workers <= 0 {
		return
	}
	if !s.queueStarted.CompareAndSwap(false, true) {
		return
	}
	if err := s.ensureQueueGroup(ctx); err != nil {
		log.Printf("ticket queue group initialization failed: %v", err)
	}
	for i := 0; i < s.queue.Workers; i++ {
		consumer := fmt.Sprintf("%s-%d", s.queue.ConsumerPrefix, i+1)
		go s.queueWorker(ctx, consumer)
	}
	log.Printf("Ticket queue workers started (stream=%s group=%s workers=%d)", s.queue.Stream, s.queue.Group, s.queue.Workers)
}

func (s *Service) ensureQueueGroup(ctx context.Context) error {
	err := s.redis.XGroupCreateMkStream(ctx, s.queue.Stream, s.queue.Group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (s *Service) queueWorker(ctx context.Context, consumer string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		streams, err := s.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    s.queue.Group,
			Consumer: consumer,
			Streams:  []string{s.queue.Stream, ">"},
			Count:    s.queue.ReadCount,
			Block:    s.queue.ReadBlock,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			log.Printf("ticket queue read error: %v", err)
			time.Sleep(time.Second)
			continue
		}
		for _, stream := range streams {
			for _, msg := range stream.Messages {
				s.processQueueMessage(ctx, msg)
			}
		}
	}
}

func (s *Service) processQueueMessage(ctx context.Context, msg redis.XMessage) {
	job, err := queueJobFromMessage(msg)
	if err != nil {
		log.Printf("invalid ticket queue message %s: %v", msg.ID, err)
		_ = s.redis.XAck(ctx, s.queue.Stream, s.queue.Group, msg.ID).Err()
		return
	}
	statusKey := queueStatusKey(job.UserID.String(), job.IdempotencyKey)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_ = s.redis.HSet(ctx, statusKey, map[string]any{
		"status":     "processing",
		"updated_at": now,
	}).Err()
	_ = s.redis.Expire(ctx, statusKey, s.queue.StatusTTL).Err()

	var app model.Application
	var created bool
	for attempt := 0; attempt < s.queue.MaxProcessAttempts; attempt++ {
		app, created, err = s.createApplicationAfterReservation(job.UserID, job.EventID, job.TicketTypeID, job.Quantity, job.IdempotencyKey)
		if err == nil || !isRetryableApplyError(err) {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 150 * time.Millisecond)
	}
	if err != nil {
		s.returnInventory(job.TicketTypeID.String(), job.Quantity)
		s.releaseQueuedUserReservation(job.UserID.String(), job.EventID.String(), job.Quantity)
		code, msgText := errorCodeAndMessage(err)
		_ = s.redis.HSet(ctx, statusKey, map[string]any{
			"status":     "failed",
			"reason":     code + ": " + msgText,
			"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
		}).Err()
		_ = s.redis.Expire(ctx, statusKey, s.queue.StatusTTL).Err()
		_ = s.redis.XAck(ctx, s.queue.Stream, s.queue.Group, msg.ID).Err()
		log.Printf("ticket queue job failed id=%s code=%s err=%v", msg.ID, code, err)
		return
	}
	_ = s.redis.HSet(ctx, statusKey, map[string]any{
		"status":         app.Status,
		"application_id": app.ID.String(),
		"updated_at":     time.Now().UTC().Format(time.RFC3339Nano),
	}).Err()
	_ = s.redis.Expire(ctx, statusKey, s.queue.StatusTTL).Err()
	_ = s.redis.XAck(ctx, s.queue.Stream, s.queue.Group, msg.ID).Err()
	if created {
		log.Printf("ticket queue job approved app=%s stream_id=%s", app.ID.String(), msg.ID)
	}
}

type queueJob struct {
	QueueID        string
	UserID         uuid.UUID
	EventID        uuid.UUID
	TicketTypeID   uuid.UUID
	Quantity       int
	IdempotencyKey string
}

func queueJobFromMessage(msg redis.XMessage) (queueJob, error) {
	get := func(key string) string {
		if v, ok := msg.Values[key]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	userID, err := uuid.Parse(get("user_id"))
	if err != nil {
		return queueJob{}, err
	}
	eventID, err := uuid.Parse(get("event_id"))
	if err != nil {
		return queueJob{}, err
	}
	ticketTypeID, err := uuid.Parse(get("ticket_type_id"))
	if err != nil {
		return queueJob{}, err
	}
	quantity, err := strconv.Atoi(get("quantity"))
	if err != nil || quantity <= 0 {
		return queueJob{}, fmt.Errorf("invalid quantity %q", get("quantity"))
	}
	return queueJob{
		QueueID:        get("queue_id"),
		UserID:         userID,
		EventID:        eventID,
		TicketTypeID:   ticketTypeID,
		Quantity:       quantity,
		IdempotencyKey: get("idempotency_key"),
	}, nil
}

func (s *Service) createApplicationAfterReservation(userID uuid.UUID, eventID uuid.UUID, ticketTypeID uuid.UUID, quantity int, idempotencyKey string) (model.Application, bool, error) {
	var createdApp model.Application
	var existingApp model.Application
	created := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		lockName := fmt.Sprintf("apply:%s:%s", userID.String(), eventID.String())
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockName).Error; err != nil {
			return err
		}
		if result := tx.Where("idempotency_key = ? AND user_id = ?", idempotencyKey, userID.String()).Limit(1).Find(&existingApp); result.Error != nil {
			return result.Error
		} else if result.RowsAffected > 0 {
			return nil
		}
		var event model.Event
		if err := tx.First(&event, "id = ?", eventID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperror.NotFound("Event not found")
			}
			return err
		}
		if event.Status != "published" {
			return apperror.New(400, "EVENT_NOT_AVAILABLE", "Event is not accepting applications")
		}
		if time.Now().After(event.ApplyDeadline) {
			return apperror.New(400, "APPLY_DEADLINE_PASSED", "Application deadline has passed")
		}
		var issuedCount int64
		if err := tx.Model(&model.Ticket{}).Where("user_id = ? AND event_id = ?", userID.String(), eventID.String()).Count(&issuedCount).Error; err != nil {
			return err
		}
		var pendingCount int
		if err := tx.Model(&model.Application{}).
			Where("user_id = ? AND event_id = ? AND status IN ?", userID.String(), eventID.String(), []string{"pending", "queued", "processing"}).
			Select("COALESCE(SUM(quantity), 0)").
			Scan(&pendingCount).Error; err != nil {
			return err
		}
		currentTotal := int(issuedCount) + pendingCount
		remainingAllowance := event.MaxTicketsPerPerson - currentTotal
		if quantity > remainingAllowance {
			return apperror.New(400, "EXCEEDS_MAX_TICKETS", fmt.Sprintf("You have already applied for %d tickets. The limit is %d. You can only apply for %d more.", currentTotal, event.MaxTicketsPerPerson, remainingAllowance))
		}
		res := tx.Model(&model.TicketType{}).
			Where("id = ? AND event_id = ? AND remaining >= ?", ticketTypeID, eventID, quantity).
			Updates(map[string]interface{}{
				"remaining": gorm.Expr("remaining - ?", quantity),
				"version":   gorm.Expr("version + 1"),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return apperror.Conflict("TICKET_SOLD_OUT", "Not enough tickets remaining")
		}
		app := model.Application{
			UserID:         userID,
			EventID:        eventID,
			TicketTypeID:   ticketTypeID,
			Quantity:       quantity,
			Status:         "approved",
			IdempotencyKey: idempotencyKey,
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "idempotency_key"}},
			DoNothing: true,
		}).Create(&app)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Model(&model.TicketType{}).Where("id = ?", ticketTypeID).Updates(map[string]interface{}{
				"remaining": gorm.Expr("remaining + ?", quantity),
				"version":   gorm.Expr("version + 1"),
			}).Error; err != nil {
				return err
			}
			if err := tx.Where("idempotency_key = ? AND user_id = ?", idempotencyKey, userID.String()).First(&existingApp).Error; err != nil {
				return err
			}
			return nil
		}
		for i := 0; i < app.Quantity; i++ {
			ticket := model.Ticket{
				ApplicationID: app.ID,
				UserID:        app.UserID,
				EventID:       app.EventID,
				TicketTypeID:  app.TicketTypeID,
				QRToken:       uuid.New().String(),
				ExpiresAt:     event.EndTime,
			}
			if err := tx.Create(&ticket).Error; err != nil {
				return err
			}
		}
		createdApp = app
		created = true
		return nil
	})
	if err != nil {
		return model.Application{}, false, err
	}
	if !created {
		return existingApp, false, nil
	}
	return createdApp, true, nil
}

func (s *Service) ensureInventoryLoaded(ctx context.Context, ticketTypeID uuid.UUID, ticketTypeIDString string) error {
	loadedKey := "inventory_loaded:" + ticketTypeIDString
	inventoryKey := "inventory:" + ticketTypeIDString
	if s.redis.Exists(ctx, loadedKey).Val() != 0 {
		return nil
	}
	lockKey := "init_lock:" + ticketTypeIDString
	ok, err := pkg.AcquireLock(ctx, s.redis, lockKey, 5*time.Second, 60*time.Second)
	if err != nil {
		return apperror.New(503, "BUSY", "Inventory system busy")
	}
	if !ok {
		return apperror.New(503, "BUSY", "Inventory system busy")
	}
	defer pkg.ReleaseLock(ctx, s.redis, lockKey)
	if s.redis.Exists(ctx, loadedKey).Val() != 0 {
		return nil
	}
	var ticketType model.TicketType
	if err := s.db.First(&ticketType, "id = ?", ticketTypeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperror.NotFound("Ticket type not found")
		}
		return apperror.Busy("Database system busy")
	}
	pipe := s.redis.TxPipeline()
	pipe.Set(ctx, inventoryKey, ticketType.Remaining, 24*time.Hour)
	pipe.Set(ctx, loadedKey, "1", 24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return apperror.New(503, "BUSY", "Inventory system busy")
	}
	return nil
}

func queuedApplicationFromStatus(userID uuid.UUID, eventID uuid.UUID, ticketTypeID uuid.UUID, req ApplyRequest, fields map[string]string) model.Application {
	appID := uuid.New()
	if fields["queue_id"] != "" {
		if parsed, err := uuid.Parse(fields["queue_id"]); err == nil {
			appID = parsed
		}
	}
	qty := req.Quantity
	if fields["quantity"] != "" {
		if parsed, err := strconv.Atoi(fields["quantity"]); err == nil {
			qty = parsed
		}
	}
	status := fields["status"]
	if status == "" {
		status = "queued"
	}
	return model.Application{
		ID:             appID,
		UserID:         userID,
		EventID:        eventID,
		TicketTypeID:   ticketTypeID,
		Quantity:       qty,
		Status:         status,
		IdempotencyKey: req.IdempotencyKey,
	}
}

func (s *Service) releaseQueuedUserReservation(userID string, eventID string, quantity int) {
	if s.redis == nil || quantity <= 0 {
		return
	}
	s.redis.DecrBy(context.Background(), queueUserEventReservationKey(userID, eventID), int64(quantity))
}

func queueStatusKey(userID string, idempotencyKey string) string {
	return "queue:status:" + userID + ":" + idempotencyKey
}

func queueReservationKey(userID string, idempotencyKey string) string {
	return "queue:reservation:" + userID + ":" + idempotencyKey
}

func queueUserEventReservationKey(userID string, eventID string) string {
	return "queue:user_event_reserved:" + userID + ":" + eventID
}

func errorCodeAndMessage(err error) (string, string) {
	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		return appErr.Code, appErr.Message
	}
	return "INTERNAL_ERROR", err.Error()
}

const queueReservationLua = `
local inventoryKey = KEYS[1]
local reservationKey = KEYS[2]
local streamKey = KEYS[3]
local statusKey = KEYS[4]
local userEventReservationKey = KEYS[5]

local quantity = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local queueID = ARGV[3]
local userID = ARGV[4]
local eventID = ARGV[5]
local ticketTypeID = ARGV[6]
local idempotencyKey = ARGV[7]
local queuedAt = ARGV[8]
local remainingAllowance = tonumber(ARGV[9])

if redis.call('EXISTS', reservationKey) == 1 then
  return 'DUPLICATE'
end

local reserved = tonumber(redis.call('GET', userEventReservationKey) or '0')
if reserved + quantity > remainingAllowance then
  return 'EXCEEDS_MAX_TICKETS'
end

local stock = redis.call('DECRBY', inventoryKey, quantity)
if stock < 0 then
  redis.call('INCRBY', inventoryKey, quantity)
  return 'SOLD_OUT'
end

redis.call('INCRBY', userEventReservationKey, quantity)
redis.call('EXPIRE', userEventReservationKey, ttl)
redis.call('SET', reservationKey, queueID, 'EX', ttl)
redis.call('HSET', statusKey,
  'status', 'queued',
  'queue_id', queueID,
  'user_id', userID,
  'event_id', eventID,
  'ticket_type_id', ticketTypeID,
  'quantity', tostring(quantity),
  'idempotency_key', idempotencyKey,
  'queued_at', queuedAt,
  'updated_at', queuedAt
)
redis.call('EXPIRE', statusKey, ttl)
redis.call('XADD', streamKey, '*',
  'queue_id', queueID,
  'user_id', userID,
  'event_id', eventID,
  'ticket_type_id', ticketTypeID,
  'quantity', tostring(quantity),
  'idempotency_key', idempotencyKey,
  'queued_at', queuedAt
)
return 'QUEUED'
`
