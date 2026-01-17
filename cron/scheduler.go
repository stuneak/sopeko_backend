package cron

import (
	"log"
	"time"

	"github.com/go-co-op/gocron/v2"
)

type Scheduler struct {
	scheduler gocron.Scheduler
}

func NewScheduler() (*Scheduler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}

	return &Scheduler{scheduler: s}, nil
}

func (s *Scheduler) RegisterJobs() error {
	// Job 1: Runs every 1 Hour
	_, err := s.scheduler.NewJob(
		gocron.DurationJob(1*time.Hour),
		gocron.NewTask(func() {
			log.Println("[CRON] Job 1: Syncing ticker prices...")
		}),
		gocron.WithName("ticker-price-sync"),
	)
	if err != nil {
		return err
	}

	// Job 2: Runs every 2 Hours

	_, err = s.scheduler.NewJob(
		gocron.DurationJob(2*time.Hour),
		gocron.NewTask(func() {
			log.Println("[CRON] Job 2: Aggregating ticker mentions...")
		}),
		gocron.WithName("mention-aggregation"),
	)
	if err != nil {
		return err
	}

	// Job 3: Runs every 3 Hour
	_, err = s.scheduler.NewJob(
		gocron.DurationJob(3*time.Hour),
		gocron.NewTask(func() {
			log.Println("[CRON] Job 3: Cleaning up old comments...")
		}),
		gocron.WithName("comment-cleanup"),
	)
	if err != nil {
		return err
	}

	// Job 4: Runs every 4 Hour
	_, err = s.scheduler.NewJob(
		gocron.DurationJob(4*time.Hour),
		gocron.NewTask(func() {
			log.Println("[CRON] Job 4: Generating health check report...")
		}),
		gocron.WithName("health-check-report"),
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Scheduler) Start() {
	log.Println("[CRON] Starting scheduler with 4 jobs...")
	s.scheduler.Start()
}

func (s *Scheduler) Stop() error {
	log.Println("[CRON] Stopping scheduler...")
	return s.scheduler.Shutdown()
}
