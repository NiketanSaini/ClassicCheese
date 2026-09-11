// Command historywriter consumes historywriter-queue and inserts every
// event into MySQL. This is the durability path, fully decoupled from the
// request path: if MySQL is briefly down, score updates still succeed and
// RabbitMQ just holds the backlog until this consumer catches up.
package main

import (
	"context"
	"encoding/json"
	"log"

	"leaderboard-poc/internal/config"
	"leaderboard-poc/internal/models"
	"leaderboard-poc/internal/mysqlclient"
	"leaderboard-poc/internal/rabbitmqclient"
)

const (
	queueName   = "historywriter-queue"
	consumePref = 10
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := mysqlclient.New(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer db.Close()

	conn, ch, err := rabbitmqclient.Connect(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	if err := rabbitmqclient.DeclareExchange(ch, cfg.RabbitMQExchange); err != nil {
		log.Fatalf("declaring exchange: %v", err)
	}
	if _, err := rabbitmqclient.DeclareAndBindQueue(ch, cfg.RabbitMQExchange, queueName); err != nil {
		log.Fatalf("declaring queue: %v", err)
	}

	deliveries, err := rabbitmqclient.Consume(ch, queueName, consumePref)
	if err != nil {
		log.Fatalf("consuming: %v", err)
	}

	ctx := context.Background()
	log.Printf("historywriter: waiting for messages on %s", queueName)

	for d := range deliveries {
		var event models.ScoreEvent
		if err := json.Unmarshal(d.Body, &event); err != nil {
			log.Printf("historywriter: dropping unparseable message: %v", err)
			d.Nack(false, false)
			continue
		}

		if err := mysqlclient.InsertScoreEvent(ctx, db, event); err != nil {
			log.Printf("historywriter: insert failed, requeueing: %v", err)
			d.Nack(false, true)
			continue
		}

		log.Printf("historywriter: recorded user=%d region=%s %s/%s delta=%d",
			event.UserID, event.Region, event.PeriodType, event.PeriodKey, event.ScoreDelta)
		d.Ack(false)
	}
}
