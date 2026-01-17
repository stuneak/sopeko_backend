package main

import (
	"log"

	"github.com/stuneak/bagger/api"
	"github.com/stuneak/bagger/config"
	"github.com/stuneak/bagger/cron"
	db "github.com/stuneak/bagger/db/sqlc"
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
	scheduler, err := cron.NewScheduler()
	if err != nil {
		log.Fatal("cannot create scheduler:", err)
	}

	err = scheduler.RegisterJobs()
	if err != nil {
		log.Fatal("cannot register cron jobs:", err)
	}

	scheduler.Start()
	defer scheduler.Stop()

	server := api.NewServer(store)

	err = server.Start(config.ServerAddress)
	if err != nil {
		log.Fatal("cannot start server:", err)
	}
}
