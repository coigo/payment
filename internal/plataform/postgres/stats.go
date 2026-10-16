package postgres

import (
	"github.com/coigo/payment-processor-course/internal/plataform/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterPoolMetrics (pool *pgxpool.Pool) {
	metrics.NewGaugeFunc(
		"pgx_pool_total_conns",
		"Connections currently open in the pool",
		func () float64 {return float64(pool.Stat().TotalConns())})

	metrics.NewGaugeFunc(
		"pgx_pool_idle_conns",
		"Idle connections in the pool",
		func () float64 {return float64(pool.Stat().IdleConns())})

	metrics.NewGaugeFunc(
		"pgx_pool_acquired_conns",
		"Acquired connections in the pool",
		func () float64 {return float64(pool.Stat().AcquiredConns())})
	
	metrics.NewGaugeFunc(
		"pgx_pool_empty_acquired_conns",
		"Empty acquired connections in the pool",
		func () float64 {return float64(pool.Stat().EmptyAcquireCount())})
}