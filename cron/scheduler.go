package cron

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	db "github.com/stuneak/bagger/db/sqlc"
)

var slog = log.New(log.Writer(), "[CRON] ", log.Flags())

var subreddits = []string{
	"pennystocks",
	"investing",
	"stocks",
	"wallstreetbets",
}

type Scheduler struct {
	scheduler     gocron.Scheduler
	store         *db.Queries
	redditScraper *RedditScraper
	nasdaqFetcher *NasdaqFetcher
	tickerParser  *TickerParser
}

func NewScheduler(db *db.Queries) (*Scheduler, error) {
	usEastern, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, err
	}

	s, err := gocron.NewScheduler(gocron.WithLocation(usEastern))
	if err != nil {
		return nil, err
	}

	return &Scheduler{
		scheduler:     s,
		store:         db,
		redditScraper: NewRedditScraper(),
		nasdaqFetcher: NewNasdaqFetcher(),
		tickerParser:  NewTickerParser(db),
	}, nil
}

func (s *Scheduler) scrapeSubreddit(subreddit string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	slog.Printf("Starting Reddit scrape for r/%s", subreddit)

	posts, comments, err := s.redditScraper.ScrapeSubreddit(ctx, subreddit)
	if err != nil {
		slog.Printf("Error scraping r/%s: %v", subreddit, err)
		return
	}

	slog.Printf("Scraped r/%s: %d posts, %d comments", subreddit, len(posts), len(comments))

	// Process posts for ticker mentions
	if err := s.tickerParser.ProcessPosts(ctx, posts); err != nil {
		slog.Printf("Error processing posts for r/%s: %v", subreddit, err)
	}

	// Process comments for ticker mentions
	if err := s.tickerParser.ProcessComments(ctx, comments); err != nil {
		slog.Printf("Error processing comments for r/%s: %v", subreddit, err)
	}

	slog.Printf("Finished processing r/%s", subreddit)
}

func (s *Scheduler) syncTickers() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	slog.Println("Starting NASDAQ tickers sync")

	stocks, err := s.nasdaqFetcher.FetchStocks(ctx)
	if err != nil {
		slog.Printf("Error fetching NASDAQ stocks: %v", err)
		return
	}

	slog.Printf("Fetched %d stocks from NASDAQ", len(stocks))

	var synced int
	for _, stock := range stocks {
		err := s.store.UpsertTicker(ctx, db.UpsertTickerParams{
			Symbol:      stock.Symbol,
			CompanyName: stock.Name,
			Exchange:    "NASDAQ",
		})
		if err != nil {
			slog.Printf("Error upserting ticker %s: %v", stock.Symbol, err)
			continue
		}
		synced++
	}

	slog.Printf("Synced %d tickers to database", synced)
}

func (s *Scheduler) fetchAllTickerPrices() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	slog.Println("Starting daily ticker prices fetch")

	tickers, err := s.store.ListAllTickers(ctx)
	if err != nil {
		slog.Printf("Error fetching tickers: %v", err)
		return
	}

	slog.Printf("Fetching prices for %d tickers", len(tickers))

	now := time.Now()
	var fetched, errors int

	for i, ticker := range tickers {
		if i > 0 && i%100 == 0 {
			slog.Printf("Progress: %d/%d tickers processed (%d errors)", i, len(tickers), errors)
		}

		price, err := s.tickerParser.FetchCurrentPrice(ctx, ticker.Symbol)
		if err != nil {
			errors++
			continue
		}

		_, err = s.store.InsertTickerPrice(ctx, db.InsertTickerPriceParams{
			TickerID:   ticker.ID,
			Price:      price,
			RecordedAt: now,
		})
		if err != nil {
			slog.Printf("Error inserting price for %s: %v", ticker.Symbol, err)
			errors++
			continue
		}
		fetched++
	}

	slog.Printf("Finished daily prices fetch: %d fetched, %d errors", fetched, errors)
}

func (s *Scheduler) RegisterJobs() error {
	// NASDAQ tickers sync - once daily
	_, err := s.scheduler.NewJob(
		gocron.DurationJob(24*time.Hour),
		gocron.NewTask(func() {
			s.syncTickers()
		}),
		gocron.WithName("nasdaq-tickers-sync"),
		gocron.WithStartAt(gocron.WithStartDateTime(time.Now().Add(5*time.Second))),
	)
	if err != nil {
		return err
	}

	slog.Println("Registered NASDAQ tickers sync job (daily)")

	// Daily ticker prices fetch - 10:00 AM US Eastern
	_, err = s.scheduler.NewJob(
		gocron.CronJob("0 10 * * *", false),
		gocron.NewTask(func() {
			s.fetchAllTickerPrices()
		}),
		gocron.WithName("daily-ticker-prices"),
	)
	if err != nil {
		return err
	}

	slog.Println("Registered daily ticker prices job (10:00 AM US Eastern)")

	// Reddit scraping jobs - staggered by 1 hour, each runs every 4 hours
	// First: 9:40, 13:40, 17:40, 21:40, 1:40, 5:40
	// Second: 10:40, 14:40, 18:40, 22:40, 2:40, 6:40
	// Third: 11:40, 15:40, 19:40, 23:40, 3:40, 7:40
	// Fourth: 12:40, 16:40, 20:40, 0:40, 4:40, 8:40
	for i, subreddit := range subreddits {
		sub := subreddit
		startHour := 9 + i

		// Generate hours for every 4 hours starting from startHour
		var hours []string
		for h := startHour; h < startHour+24; h += 4 {
			hours = append(hours, fmt.Sprintf("%d", h%24))
		}
		hourList := strings.Join(hours, ",")

		_, err := s.scheduler.NewJob(
			gocron.CronJob(
				fmt.Sprintf("40 %s * * *", hourList),
				false,
			),
			gocron.NewTask(func() {
				s.scrapeSubreddit(sub)
			}),
			gocron.WithName("reddit-scrape-"+sub),
		)
		if err != nil {
			return err
		}

		slog.Printf("Registered Reddit scrape job for r/%s (every 4h at :40, starting %d:40 US Eastern)", sub, startHour)
	}

	return nil
}

func (s *Scheduler) Start() {
	slog.Printf("Starting scheduler with %d Reddit scraping jobs...", len(subreddits))
	s.scheduler.Start()
}

func (s *Scheduler) Stop() error {
	slog.Println("Stopping scheduler...")
	return s.scheduler.Shutdown()
}
