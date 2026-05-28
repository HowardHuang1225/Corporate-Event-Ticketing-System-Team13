package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	ticketRemainingGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "event_ticket_remaining",
			Help: "Remaining tickets for each event",
		},
		[]string{"event_id"},
	)

	ticketApplyTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ticket_apply_total",
			Help: "Total ticket apply requests per event",
		},
		[]string{"event_id"},
	)

	ticketRedeemTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ticket_redeem_total",
			Help: "Total ticket redeem (check-in) per event",
		},
		[]string{"event_id"},
	)

	registerOnce sync.Once
)

func Register() {
	registerOnce.Do(func() {
		prometheus.MustRegister(ticketRemainingGauge)
		prometheus.MustRegister(ticketApplyTotal)
		prometheus.MustRegister(ticketRedeemTotal)
	})
}

func SetTicketRemaining(eventID string, remaining int) {
	ticketRemainingGauge.WithLabelValues(eventID).Set(float64(remaining))
}

func IncTicketApply(eventID string) {
	ticketApplyTotal.WithLabelValues(eventID).Inc()
}

func IncTicketRedeem(eventID string) {
	ticketRedeemTotal.WithLabelValues(eventID).Inc()
}
