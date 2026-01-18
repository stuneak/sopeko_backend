package main

import (
	"log"

	"github.com/stuneak/sopeko/api"
	"github.com/stuneak/sopeko/config"
	"github.com/stuneak/sopeko/cron"
	db "github.com/stuneak/sopeko/db/sqlc"
)

func main() {
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatal("cannot load config:", err)
	}

	conn, err := db.NewDB(config.DBDriver, config.DBSource)
	if err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	store := db.New(conn)

	// Initialize and start cron scheduler
	scheduler, err := cron.NewScheduler(store)
	if err != nil {
		log.Fatal("cannot create scheduler:", err)
	}

	err = scheduler.RegisterJobs()
	if err != nil {
		log.Fatal("cannot register cron jobs:", err)
	}

	scheduler.Start()
	defer scheduler.Stop()

	server := api.NewServer(store, config.GINMode)

	err = server.Start(config.ServerAddress)
	if err != nil {
		log.Fatal("cannot start server:", err)
	}
}
