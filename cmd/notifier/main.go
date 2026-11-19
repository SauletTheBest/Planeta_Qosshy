// notifier reads the shop's order events from Kafka and emails the customer:
// "оплачен", "отправлен", "доставлен", "отменён", "деньги возвращены".
//
// It is a separate process from the shop: if it is stopped, nobody notices at checkout;
// the events simply wait in Kafka, and when it starts again it continues from the last
// message it confirmed (its offset in the consumer group "notifier").
//
// Run (the docker-compose service "notifier" does this):
//
//	DATABASE_URL=… KAFKA_BROKERS=kafka:29092 SMTP_HOST=… ./notifier
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"planeta_qosshy/events"
	"planeta_qosshy/middleware"
	"planeta_qosshy/notify"
	"planeta_qosshy/util"

	"github.com/joho/godotenv"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const consumerGroup = "notifier"

func main() {
	_ = godotenv.Load()
	middleware.SetupLogger()
	log := middleware.Log()

	dsn := os.Getenv("DATABASE_URL")
	brokers := util.SplitList(os.Getenv("KAFKA_BROKERS"))
	if dsn == "" || len(brokers) == 0 {
		log.Fatal("DATABASE_URL and KAFKA_BROKERS must be set")
	}

	// The notifier only reads and writes its own table; the schema (migrations) belongs
	// to the shop, so it is NOT migrated from here.
	var db *gorm.DB
	var err error
	for i := 1; i <= 10; i++ {
		if db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)}); err == nil {
			break
		}
		log.Warnf("notifier: database not ready (%d/10): %v", i, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("notifier: database: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   events.TopicOrderEvents,
		// A consumer group: Kafka remembers the offset — how far "notifier" has read.
		// Several notifier copies in one group would share the partitions between them.
		GroupID: consumerGroup,
		// A brand-new group starts from the oldest message still in the topic.
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    1 << 20,
		MaxWait:     time.Second,
		// 0 = commit right away when we call CommitMessages (we do it after each letter).
		CommitInterval: 0,
	})
	defer reader.Close()

	shopURL := os.Getenv("PUBLIC_URL")
	if shopURL == "" {
		shopURL = "http://localhost:8080"
	}
	proc := &notify.Processor{
		DB:      db,
		Send:    util.SendEmail,
		ShopURL: shopURL,
		Retries: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second},
		Log:     log,
	}

	log.Infof("notifier: reading %q as group %q from %v", events.TopicOrderEvents, consumerGroup, brokers)
	for {
		// FetchMessage, not ReadMessage: ReadMessage would confirm the message BEFORE we
		// sent the letter, and a crash in between would lose it.
		m, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break // shutting down
			}
			log.Warnf("notifier: fetch: %v", err)
			time.Sleep(time.Second)
			continue
		}

		// Retry this very message until it is handled (e.g. the DB was restarting):
		// moving on would skip it for good.
		for attempt := 0; ; attempt++ {
			err := proc.Handle(ctx, m.Value)
			if err == nil || ctx.Err() != nil {
				break
			}
			pause := min(time.Duration(1<<min(attempt, 5))*time.Second, 30*time.Second)
			log.Warnf("notifier: offset %d: %v (retry in %s)", m.Offset, err, pause)
			time.Sleep(pause)
		}
		if ctx.Err() != nil {
			break // not committed: the message is read again after the restart
		}

		// Tell Kafka "done up to here": after a restart reading continues after it.
		if err := reader.CommitMessages(ctx, m); err != nil && ctx.Err() == nil {
			log.Warnf("notifier: commit offset %d: %v", m.Offset, err)
		}
	}
	log.Info("notifier: stopped")
}
