// Command api is the HTTP + WebSocket front door: synchronous reads/writes
// against Redis and MySQL, plus fire-and-forget publishes to RabbitMQ so
// durability, aggregation, and notifications happen out-of-band in other
// processes.
package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	"leaderboard-poc/internal/config"
	"leaderboard-poc/internal/mysqlclient"
	"leaderboard-poc/internal/rabbitmqclient"
	"leaderboard-poc/internal/redisclient"
)

type server struct {
	cfg      *config.Config
	rdb      *redis.Client
	db       *sql.DB
	amqpConn *amqp.Connection
	amqpCh   *amqp.Channel
	hub      *Hub
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	rdb := redisclient.New(cfg.RedisAddr)
	defer rdb.Close()

	db, err := mysqlclient.New(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.Close()

	amqpConn, amqpCh, err := rabbitmqclient.Connect(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer amqpConn.Close()
	defer amqpCh.Close()

	if err := rabbitmqclient.DeclareExchange(amqpCh, cfg.RabbitMQExchange); err != nil {
		log.Fatalf("declaring exchange: %v", err)
	}

	srv := &server{
		cfg:      cfg,
		rdb:      rdb,
		db:       db,
		amqpConn: amqpConn,
		amqpCh:   amqpCh,
		hub:      NewHub(rdb),
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Post("/score/update", srv.handleScoreUpdate)
	r.Get("/leaderboard/top", srv.handleLeaderboardTop)
	r.Get("/leaderboard/rank", srv.handleLeaderboardRank)
	r.Get("/leaderboard/around", srv.handleLeaderboardAround)
	r.Get("/leaderboard/history", srv.handleLeaderboardHistory)
	r.Post("/admin/snapshot", srv.handleAdminSnapshot)
	r.Get("/ws/leaderboard", srv.handleWebSocket)

	addr := ":" + cfg.HTTPPort
	log.Printf("api listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("http server: %v", err)
	}
}
