package handler

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type invalidEventTest struct {
	name                string
	progress            string
	mutate              func(gin.H)
	wantMessageContains string
}

func CreateTimeInvalidEvent(t *testing.T, errs *testErrors, curTime time.Time) []invalidEventTest {
	t.Helper()

	timeRelatedTests := []invalidEventTest{
		{
			name:     	"publish time after start time",
			progress: 	"test if an event with publish_time after start_time is rejected.",
			mutate: func(payload gin.H) {
				payload["publish_time"] = curTime.Add(73 * time.Hour).Format(time.RFC3339)
			},
			wantMessageContains: "publish_time <= start_time < apply_deadline <= end_time",
		},
		{
			name:     	"start time not before apply deadline",
			progress: 	"test if an event with start_time not before apply_deadline is rejected.",
			mutate: func(payload gin.H) {
				payload["apply_deadline"] = curTime.Add(72 * time.Hour).Format(time.RFC3339)
			},
			wantMessageContains: "publish_time <= start_time < apply_deadline <= end_time",
		},
		{
			name:     	"apply deadline after end time",
			progress: 	"test if an event with apply_deadline after end_time is rejected.",
			mutate: func(payload gin.H) {
				payload["apply_deadline"] = curTime.Add(77 * time.Hour).Format(time.RFC3339)
			},
			wantMessageContains: "publish_time <= start_time < apply_deadline <= end_time",
		},
	}

	return timeRelatedTests
}

func CreateTicketInvalidEvent(t *testing.T, errs *testErrors) []invalidEventTest {
	t.Helper()

	ticketRelatedTests := []invalidEventTest{
		{
			name:     	"no ticket types",
			progress: 	"test if creating an event without ticket types is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []any{}
			},
			wantMessageContains: "ticket_types must have at least 1 item",
		},
		{
			name:     	"ticket type no exist",
			progress: 	"test if creating an event with no ticket types is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = nil
			},
			wantMessageContains: "at least one ticket type is required",
		},

		{
			name:     	"ticket type without name",
			progress: 	"test if creating an event with a ticket type that is missing the required name field is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"total_quota": 10},
				}
			},
			wantMessageContains: "Key: 'CreateEventRequest.TicketTypes[0].Name' Error:Field validation for 'Name' failed on the 'required' tag",
		},
		{
			name:     	"ticket type with missing total_quota",
			progress: 	"test if creating an event with a ticket type that is missing the required total_quota field is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "Invalid Ticket"},
				}
			},
			wantMessageContains: "Key: 'CreateEventRequest.TicketTypes[0].TotalQuota' Error:Field validation for 'TotalQuota' failed on the 'required' tag",
		},
		{
			name:     	"ticket type with missing name and quota",
			progress: 	"test if creating an event with a ticket type that is missing both the required name and total_quota fields is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{},
				}
			},
			wantMessageContains: "Key: 'CreateEventRequest.TicketTypes[0].Name' Error:Field validation for 'Name' failed on the 'required' tag; Key: 'CreateEventRequest.TicketTypes[0].TotalQuota' Error:Field validation for 'TotalQuota' failed on the 'required' tag",
		},
		{
			name:		"ticket type with null name and quota",
			progress: 	"test if creating an event with a ticket type that has null name and quota is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					nil,
				}
			},
			wantMessageContains: "json: cannot unmarshal null into Go struct field CreateEventRequest.TicketTypes.name",
		},

		{
			name:     	"ticket type with null name",
			progress: 	"test if creating an event with a ticket type that has a null name is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": nil, "total_quota": 10},
				}
			},
			wantMessageContains: "json: cannot unmarshal null into Go struct field CreateEventRequest.TicketTypes.name",
		},
		{
			name:     	"ticket type with empty name",
			progress: "	test if creating an event with a ticket type that has an empty name is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "", "total_quota": 10},
				}
			},
			wantMessageContains: "Key: 'CreateEventRequest.TicketTypes[0].Name' Error:Field validation for 'Name' failed on the 'required' tag",
		},
		{
			name:     	"ticket type with name that has only spaces",
			progress: 	"test if creating an event with a ticket type that has a name consisting of only spaces is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "   ", "total_quota": 10},
				}
			},
			wantMessageContains: "Key: 'CreateEventRequest.TicketTypes[0].Name' Error:Field validation for 'Name' failed on the 'required' tag",
		},
		{
			name:     	"duplicate ticket type names",
			progress: 	"test if creating an event with duplicate ticket type names is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "Duplicate Ticket", "total_quota": 10},
					{"name": "Duplicate Ticket", "total_quota": 20},
				}
			},
			wantMessageContains: "duplicate ticket type names are not allowed",
		},
		{
			name:		"ticket type with name isn't string",
			progress:	"test if creating an event with a ticket type that has a name that isn't a string is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": 123, "total_quota": 10},
				}
			},
			wantMessageContains: "json: cannot unmarshal number into Go struct field CreateEventRequest.TicketTypes.name",
		},

		{
			name:     	"ticket type with null quota",
			progress: 	"test if creating an event with a ticket type that has a null total_quota is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "Invalid Ticket", "total_quota": nil},
				}
			},
			wantMessageContains: "json: cannot unmarshal null into Go struct field CreateEventRequest.TicketTypes.total_quota",
		},
		{
			name:     	"ticket type with zero quota",
			progress: 	"test if creating an event with a ticket type that has zero total_quota is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "Invalid Ticket", "total_quota": 0},
				}
			},
			wantMessageContains: "total_quota must be greater than 0",
		},
		{
			name:     	"ticket type with negative quota",
			progress: 	"test if creating an event with a ticket type that has negative total_quota is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "Invalid Ticket", "total_quota": -5},
				}
			},
			wantMessageContains: "total_quota must be greater than 0",
		},
		{
			name:     	"ticket type with floating point quota",
			progress: 	"test if creating an event with a ticket type that has a floating point total_quota is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "Invalid Ticket", "total_quota": 10.5},
				}
			},
			wantMessageContains: "json: cannot unmarshal number into Go struct field CreateEventRequest.TicketTypes.total_quota",
		},
		{
			name:     	"ticket type with non-integer quota",
			progress: 	"test if creating an event with a ticket type that has a non-integer total_quota is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "Invalid Ticket", "total_quota": "ten"},
				}
			},
			wantMessageContains: "json: cannot unmarshal string into Go struct field CreateEventRequest.TicketTypes.total_quota",
		},
		
		{
			name:     	"ticket type with extra fields",
			progress: 	"test if creating an event with a ticket type that has extra fields is rejected.",
			mutate: func(payload gin.H) {
				payload["ticket_types"] = []gin.H{
					{"name": "Valid Ticket", "total_quota": 10},
					{"name": "Invalid Ticket", "total_quota": 10, "extra_field": "not allowed"},
				}
			},
			wantMessageContains: "json: unknown field \"extra_field\"",
		},		
	}

	return ticketRelatedTests
}

func CreateValueInvalidEvent(t *testing.T, errs *testErrors) []invalidEventTest {
	t.Helper()

	numberRelatedTests := []invalidEventTest{
		{
			name:		"null max tickets per person",
			progress:	"test if creating an event with a null max_tickets_per_person is rejected.",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = nil
			},
			wantMessageContains: "json: cannot unmarshal null into Go struct field CreateEventRequest.max_tickets_per_person",
		},
		{
			name:     	"zero max tickets per person",
			progress: 	"test if max_tickets_per_person equal to zero is rejected.",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = 0
			},
			wantMessageContains: "max_tickets_per_person must be greater than 0",
		},
		{
			name:     	"negative max tickets per person",
			progress: 	"test if negative max_tickets_per_person is rejected.",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = -1
			},
			wantMessageContains: "max_tickets_per_person must be greater than 0",
		},
		{
			name:     	"floating point max tickets per person",
			progress: 	"test if creating an event with a floating point max_tickets_per_person is rejected.",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = 2.5
			},
			wantMessageContains: "json: cannot unmarshal number into Go struct field CreateEventRequest.max_tickets_per_person",
		},
		{
			name:     	"max_tickets_per_person with non-integer value",
			progress: 	"test if creating an event with a non-integer max_tickets_per_person is rejected.",
			mutate: func(payload gin.H) {
				payload["max_tickets_per_person"] = "two"
			},
			wantMessageContains: "json: cannot unmarshal string into Go struct field CreateEventRequest.max_tickets_per_person",
		},
	}

	return numberRelatedTests
}

func CreateStatusInvalidEvent(t *testing.T, errs *testErrors) []invalidEventTest {
	t.Helper()

	statusRelatedTests := []invalidEventTest{
		{
			name:		"null status",
			progress:	"test if creating an event with a null status is rejected.",
			mutate: func(payload gin.H) {
				payload["status"] = nil
			},
			wantMessageContains: "json: cannot unmarshal null into Go struct field CreateEventRequest.status",
		},
		{
			name:     	"empty status",
			progress: 	"test if creating an event with an empty status is rejected.",
			mutate: func(payload gin.H) {
				payload["status"] = ""
			},
			wantMessageContains: "invalid event status",
		},
		{
			name:		"status with only spaces",
			progress:	"test if creating an event with a status that has only spaces is rejected.",
			mutate: func(payload gin.H) {
				payload["status"] = "   "
			},
			wantMessageContains: "invalid event status",
		},
		{
			name:     	"uppercase status",
			progress: 	"test if creating an event with an uppercase status is rejected.",
			mutate: func(payload gin.H) {
				payload["status"] = "DRAFT"
			},
			wantMessageContains: "invalid event status",
		},
		{
			name:     	"closed status",
			progress: 	"test if creating an event with closed status is rejected.",
			mutate: func(payload gin.H) {
				payload["status"] = "closed"
			},
			wantMessageContains: "event status cannot be closed or ended",
		},
		{
			name:     	"ended status",
			progress: 	"test if creating an event with ended status is rejected.",
			mutate: func(payload gin.H) {
				payload["status"] = "ended"
			},
			wantMessageContains: "event status cannot be closed or ended",
		},

		{
			name:     	"invalid status",
			progress: 	"test if creating an event with an invalid status is rejected.",
			mutate: func(payload gin.H) {
				payload["status"] = "invalid_status"
			},
			wantMessageContains: "invalid event status",
		},

		{
			name:     	"status with spaces",
			progress: 	"test if creating an event with a status that has leading/trailing spaces is rejected.",
			mutate: func(payload gin.H) {
				payload["status"] = " draft "
			},
			wantMessageContains: "invalid event status",
		},		
	}

	return statusRelatedTests
}
