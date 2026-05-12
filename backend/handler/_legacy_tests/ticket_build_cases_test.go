package handler

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
)

type invalidApplyQuantityTest struct {
	name         string
	progress     string
	quantity     any
	omitQuantity bool
}

func buildInvalidApplyQuantityTests() []invalidApplyQuantityTest {
	return []invalidApplyQuantityTest{
		{
			name:         "missing quantity",
			progress:     "submitting an application without quantity",
			omitQuantity: true,
		},
		{
			name:     "null quantity",
			progress: "submitting an application with quantity set to null",
			quantity: nil,
		},
		{
			name:     "zero quantity",
			progress: "submitting an application with quantity equal to zero",
			quantity: 0,
		},
		{
			name:     "negative quantity",
			progress: "submitting an application with a negative quantity",
			quantity: -1,
		},
		{
			name:     "fractional quantity",
			progress: "submitting an application with a fractional quantity",
			quantity: 1.5,
		},
		{
			name:     "decimal notation quantity",
			progress: "submitting an application with decimal notation",
			quantity: json.Number("1.0"),
		},
		{
			name:     "string quantity",
			progress: "submitting an application with quantity as a string",
			quantity: "1",
		},
		{
			name:     "boolean quantity",
			progress: "submitting an application with quantity as a boolean",
			quantity: true,
		},
		{
			name:     "array quantity",
			progress: "submitting an application with quantity as an array",
			quantity: []int{1},
		},
		{
			name:     "object quantity",
			progress: "submitting an application with quantity as an object",
			quantity: gin.H{"value": 1},
		},
	}
}

type invalidApplyStatusTest struct {
	name       string
	progress   string
	status     any
	omitStatus bool
}

func buildInvalidApplyStatusTests() []invalidApplyStatusTest {
	return []invalidApplyStatusTest{
		{
			name:       "missing status",
			progress:   "submitting an application without status",
			omitStatus: true,
		},
		{
			name:     "null status",
			progress: "submitting an application with status set to null",
			status:   nil,
		},
		{
			name:     "empty status",
			progress: "submitting an application with an empty status",
			status:   "",
		},
		{
			name:     "spaces only status",
			progress: "submitting an application with a status that contains only spaces",
			status:   "   ",
		},
		{
			name:     "tabs and newlines only status",
			progress: "submitting an application with a status that contains only tabs and newlines",
			status:   "\t\n",
		},
		{
			name:     "duplicate pending status words",
			progress: "submitting an application with repeated pending status words",
			status:   "pending pending",
		},
		{
			name:     "duplicate pending status comma separated",
			progress: "submitting an application with repeated pending status values separated by a comma",
			status:   "pending,pending",
		},
		{
			name:     "number status",
			progress: "submitting an application with status as a number",
			status:   1,
		},
		{
			name:     "boolean status",
			progress: "submitting an application with status as a boolean",
			status:   true,
		},
		{
			name:     "array status",
			progress: "submitting an application with status as an array",
			status:   []string{"pending"},
		},
		{
			name:     "object status",
			progress: "submitting an application with status as an object",
			status:   gin.H{"value": "pending"},
		},
		{
			name:     "approved status",
			progress: "submitting an employee application with an approved status",
			status:   "approved",
		},
		{
			name:     "rejected status",
			progress: "submitting an employee application with a rejected status",
			status:   "rejected",
		},
		{
			name:     "cancelled status",
			progress: "submitting an employee application with a cancelled status",
			status:   "cancelled",
		},
		{
			name:     "draft event status",
			progress: "submitting an employee application with an event-only draft status",
			status:   "draft",
		},
		{
			name:     "published event status",
			progress: "submitting an employee application with an event-only published status",
			status:   "published",
		},
		{
			name:     "closed event status",
			progress: "submitting an employee application with an event-only closed status",
			status:   "closed",
		},
		{
			name:     "uppercase pending status",
			progress: "submitting an application with an uppercase pending status",
			status:   "PENDING",
		},
		{
			name:     "mixed case pending status",
			progress: "submitting an application with a mixed case pending status",
			status:   "Pending",
		},
		{
			name:     "pending with surrounding spaces",
			progress: "submitting an application with pending surrounded by spaces",
			status:   " pending ",
		},
		{
			name:     "pending with newline suffix",
			progress: "submitting an application with pending followed by a newline",
			status:   "pending\n",
		},
		{
			name:     "unknown status",
			progress: "submitting an application with an unknown status",
			status:   "waitlisted",
		},
		{
			name:     "sql-like status",
			progress: "submitting an application with SQL-like text in status",
			status:   "pending'; DROP TABLE applications;--",
		},
		{
			name:     "very long status",
			progress: "submitting an application with an excessively long status",
			status:   strings.Repeat("pending", 64),
		},
	}
}
