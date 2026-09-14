package observability

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// poolStatter is the slice of *pgxpool.Pool these gauges read. An interface rather than the
// concrete type so the collector can be driven in a test without opening a database.
type poolStatter interface {
	Stat() *pgxpool.Stat
}

// poolCollector publishes the connection pool's occupancy.
//
// Nothing in this repository read pool.Stat() before the 2026-09-14 outage, which is the
// reason that outage was invisible. A deep-offset crawl held all ten of the API's connections
// for minutes at a time; every other request queued for one, and the signals that existed all
// looked clean — pool.Ping answers in microseconds while every connection is held, and the
// error fraction counts only responses the process PRODUCED, of which there were almost none
// because nothing was finishing.
//
// Implemented as a Collector rather than gauges a goroutine polls: Stat() reads counters the
// pool already keeps in memory, so reading them at scrape time is both cheaper and fresher
// than sampling on a timer, and there is no interval to pick or ticker to stop.
type poolCollector struct {
	pool poolStatter

	acquired  *prometheus.Desc
	idle      *prometheus.Desc
	max       *prometheus.Desc
	emptyWait *prometheus.Desc
}

// NewPoolCollector builds the collector for one pool. Register it on the default registry
// with prometheus.MustRegister; cmd/server does.
func NewPoolCollector(pool poolStatter) prometheus.Collector {
	return &poolCollector{
		pool: pool,
		acquired: prometheus.NewDesc("freehire_db_pool_acquired_connections",
			"Connections currently held by a caller.", nil, nil),
		idle: prometheus.NewDesc("freehire_db_pool_idle_connections",
			"Connections open and free to be handed out.", nil, nil),
		max: prometheus.NewDesc("freehire_db_pool_max_connections",
			"The pool's ceiling. Saturation is acquired/max; alert on the ratio, never on the raw count — the ceiling is per-process configuration and changes.", nil, nil),
		// The one that names the bottleneck rather than describing it. acquired == max is a
		// pool that is BUSY; this counter only rises when a caller found nothing free and
		// had to wait, which is a pool that is TOO SMALL for what is being asked of it. A
		// pool can sit at its ceiling all day serving fast queries without this moving.
		emptyWait: prometheus.NewDesc("freehire_db_pool_empty_acquire_total",
			"Acquisitions that found no free connection and had to wait for one.", nil, nil),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquired
	ch <- c.idle
	ch <- c.max
	ch <- c.emptyWait
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(stat.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(stat.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.max, prometheus.GaugeValue, float64(stat.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.emptyWait, prometheus.CounterValue, float64(stat.EmptyAcquireCount()))
}
