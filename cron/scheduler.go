package cron

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	external_api "github.com/stuneak/sopeko/cron/external_api"
	db "github.com/stuneak/sopeko/db/sqlc"
)

var baseLogger = log.New(log.Writer(), "[CRON] ", log.Flags())

func clog(format string, args ...interface{}) {
	pc, _, _, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(pc).Name()
	if i := strings.LastIndex(fn, "."); i >= 0 {
		fn = fn[i+1:]
	}
	baseLogger.Printf(fn+": "+format, args...)
}

var subreddits = []string{
	"pennystocks",
	"investing",
	"stocks",
}

type Scheduler struct {
	scheduler     gocron.Scheduler
	store         *db.Queries
	redditScraper *external_api.RedditScraper
	nasdaqFetcher *external_api.NasdaqFetcher
	yahooFetcher  *external_api.YahooFetcher
}

func NewScheduler(store *db.Queries) (*Scheduler, error) {
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
		store:         store,
		redditScraper: external_api.NewRedditScraper(),
		nasdaqFetcher: external_api.NewNasdaqFetcher(),
		yahooFetcher:  external_api.NewYahooFetcher(),
	}, nil
}

func (s *Scheduler) fetchSubreddit(subreddit string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	clog("starting r/%s", subreddit)

	posts, comments, err := s.redditScraper.ScrapeSubreddit(ctx, subreddit)
	if err != nil {
		clog("error scraping r/%s: %v", subreddit, err)
		return
	}

	clog("scraped r/%s: %d posts, %d comments", subreddit, len(posts), len(comments))

	// Process posts as comments (title + selftext)
	for _, post := range posts {
		if post.Author == "" || post.Author == "[deleted]" {
			continue
		}
		s.processRedditContent(ctx, post.Author, post.ID, post.Title+" "+post.Selftext, post.CreatedAt, "reddit")
	}

	// Process comments
	for _, comment := range comments {
		if comment.Author == "" || comment.Author == "[deleted]" {
			continue
		}
		s.processRedditContent(ctx, comment.Author, comment.ID, comment.Body, comment.CreatedAt, "reddit")
	}

	clog("finished r/%s", subreddit)
}

func (s *Scheduler) processRedditContent(ctx context.Context, author, externalID, content string, createdAt time.Time, source string) {
	clog("processing author=%s externalID=%s source=%s", author, externalID, source)

	// Upsert user
	user, err := s.store.GetUserByUsername(ctx, author)
	if err != nil {
		user, err = s.store.CreateUser(ctx, author)
		if err != nil {
			clog("error creating user %s: %v", author, err)
			return
		}
		clog("created new user %s", author)
	}

	// Check if comment already exists
	_, err = s.store.GetCommentByUserAndExternalID(ctx, db.GetCommentByUserAndExternalIDParams{
		UserID:     user.ID,
		ExternalID: externalID,
	})
	if err == nil {
		clog("comment already exists externalID=%s, skipping", externalID)
		return
	}

	// Create comment
	comment, err := s.store.CreateComment(ctx, db.CreateCommentParams{
		UserID:     user.ID,
		Source:     source,
		ExternalID: externalID,
		Content:    content,
		CreatedAt:  createdAt,
	})
	if err != nil {
		clog("error creating comment externalID=%s: %v", externalID, err)
		return
	}

	// Extract tickers and create mentions
	tickers := external_api.ExtractTickers(content)
	clog("extracted %d tickers from externalID=%s", len(tickers), externalID)

	for _, symbol := range tickers {
		ticker, err := s.store.GetTickerBySymbol(ctx, symbol)
		if err != nil {
			clog("ticker %s not in database, skipping", symbol)
			continue
		}

		// Ensure a ticker price exists before createdAt
		_, err = s.store.GetTickerPriceBeforeDate(ctx, db.GetTickerPriceBeforeDateParams{
			TickerID:   ticker.ID,
			RecordedAt: createdAt,
		})
		if err != nil {
			// No price found, fetch from Yahoo and store
			clog("no price for %s before %s, fetching from Yahoo", symbol, createdAt.Format("2006-01-02"))
			price, volume, recordedAt, fetchErr := s.yahooFetcher.FetchHistoricalPrice(ctx, symbol, createdAt)
			if fetchErr == nil {
				s.store.InsertTickerPrice(ctx, db.InsertTickerPriceParams{
					TickerID:   ticker.ID,
					Price:      fmt.Sprintf("%.2f", price),
					Volume:     volume,
					RecordedAt: recordedAt,
				})
				clog("stored historical price for %s: %.2f", symbol, price)
			} else {
				clog("failed to fetch historical price for %s: %v", symbol, fetchErr)
			}
		}

		s.store.CreateTickerMention(ctx, db.CreateTickerMentionParams{
			TickerID:    ticker.ID,
			UserID:      user.ID,
			CommentID:   comment.ID,
			MentionedAt: createdAt,
		})
		clog("created mention for %s by %s", symbol, author)
	}
}

func (s *Scheduler) fetchTickerNames() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	clog("starting NASDAQ tickers sync")

	stocks, err := s.nasdaqFetcher.FetchTickers(ctx)
	if err != nil {
		clog("error fetching NASDAQ stocks: %v", err)
		return
	}

	clog("fetched %d stocks from NASDAQ", len(stocks))

	var synced, skipped int
	for _, stock := range stocks {
		if strings.Contains(stock.Symbol, "^") || strings.Contains(stock.Symbol, "/") {
			skipped++
			continue
		}
		err := s.store.UpsertTicker(ctx, db.UpsertTickerParams{
			Symbol:      stock.Symbol,
			CompanyName: stock.Name,
			Exchange:    "NASDAQ",
		})
		if err != nil {
			clog("error upserting ticker %s: %v", stock.Symbol, err)
			continue
		}
		synced++
	}

	clog("done - synced %d, skipped %d (contained ^ or /)", synced, skipped)
}

func (s *Scheduler) fetchTickerPrices() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	clog("starting ticker prices fetch")

	tickers, err := s.store.ListAllTickers(ctx)
	if err != nil {
		clog("error fetching tickers from DB: %v", err)
		return
	}

	clog("fetching prices for %d tickers", len(tickers))

	var fetched int
	var errorSymbols []string

	for i, ticker := range tickers {
		if strings.Contains(ticker.Symbol, "^") || strings.Contains(ticker.Symbol, "/") {
			continue
		}

		if i > 0 && i%100 == 0 {
			clog("progress %d/%d (%d fetched, %d errors)", i, len(tickers), fetched, len(errorSymbols))
		}

		price, volume, recordedAt, err := s.yahooFetcher.FetchCurrentPriceAndVolume(ctx, ticker.Symbol)
		if err != nil {
			if len(errorSymbols) < 10 {
				clog("error for %s: %v", ticker.Symbol, err)
			}
			errorSymbols = append(errorSymbols, ticker.Symbol)
			continue
		}

		// Delete existing price for this ticker on the same day before inserting
		s.store.DeleteTickerPriceByDate(ctx, db.DeleteTickerPriceByDateParams{
			TickerID: ticker.ID,
			Date:     recordedAt,
		})

		_, err = s.store.InsertTickerPrice(ctx, db.InsertTickerPriceParams{
			TickerID:   ticker.ID,
			Price:      fmt.Sprintf("%.2f", price),
			Volume:     volume,
			RecordedAt: recordedAt,
		})
		if err != nil {
			clog("error inserting price for %s: %v", ticker.Symbol, err)
			errorSymbols = append(errorSymbols, ticker.Symbol)
			continue
		}
		fetched++
	}

	clog("done - %d fetched, %d errors", fetched, len(errorSymbols))
}

func (s *Scheduler) fetchTickerSplits() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	clog("starting ticker splits fetch")

	tickers, err := s.store.ListAllTickers(ctx)
	if err != nil {
		clog("error fetching tickers from DB: %v", err)
		return
	}

	clog("processing %d tickers for splits", len(tickers))

	var fetched, fetchErrors, insertErrors int
	for i, ticker := range tickers {
		if strings.Contains(ticker.Symbol, "^") || strings.Contains(ticker.Symbol, "/") {
			continue
		}

		if i > 0 && i%100 == 0 {
			clog("progress %d/%d (%d splits, %d fetch errors, %d insert errors)", i, len(tickers), fetched, fetchErrors, insertErrors)
		}

		splits, err := s.yahooFetcher.FetchSplits(ctx, ticker.Symbol)
		if err != nil {
			fetchErrors++
			if fetchErrors <= 10 {
				clog("error for %s: %v", ticker.Symbol, err)
			}
			continue
		}

		for _, split := range splits {
			err = s.store.InsertTickerSplit(ctx, db.InsertTickerSplitParams{
				TickerID:      ticker.ID,
				Ratio:         fmt.Sprintf("%.4f", split.Ratio),
				EffectiveDate: split.EffectiveDate,
			})
			if err != nil {
				insertErrors++
				continue
			}
			fetched++
		}
	}

	clog("done - %d splits stored, %d fetch errors, %d insert errors", fetched, fetchErrors, insertErrors)
}

func (s *Scheduler) RegisterJobs() error {
	now := time.Now()

	// 1. NASDAQ tickers sync - on startup + every 24h
	tickerSyncStart := now.Add(5 * time.Second)
	_, err := s.scheduler.NewJob(
		gocron.DurationJob(24*time.Hour),
		gocron.NewTask(s.fetchTickerNames),
		gocron.WithName("nasdaq-tickers-sync"),
		gocron.WithStartAt(gocron.WithStartDateTime(tickerSyncStart)),
	)
	if err != nil {
		return err
	}

	// 2. Ticker prices - +5 hours after startup, every 6h
	pricesStart := now.Add(5 * time.Hour)
	_, err = s.scheduler.NewJob(
		gocron.DurationJob(6*time.Hour),
		gocron.NewTask(s.fetchTickerPrices),
		gocron.WithName("ticker-prices"),
		gocron.WithStartAt(gocron.WithStartDateTime(pricesStart)),
	)
	if err != nil {
		return err
	}

	// 3. Ticker splits - +5 min after startup, every 24h
	splitsStart := now.Add(5 * time.Minute)
	_, err = s.scheduler.NewJob(
		gocron.DurationJob(24*time.Hour),
		gocron.NewTask(s.fetchTickerSplits),
		gocron.WithName("ticker-splits"),
		gocron.WithStartAt(gocron.WithStartDateTime(splitsStart)),
	)
	if err != nil {
		return err
	}

	// 4. Reddit scraping - 3h cycle, staggered: #1 at +15m, #2 at +1h, #3 at +2h
	redditDelays := []time.Duration{15 * time.Minute, 1 * time.Hour, 2 * time.Hour}
	for i, subreddit := range subreddits {
		sub := subreddit
		subStart := now.Add(redditDelays[i])

		_, err := s.scheduler.NewJob(
			gocron.DurationJob(3*time.Hour),
			gocron.NewTask(func() { s.fetchSubreddit(sub) }),
			gocron.WithName("reddit-scrape-"+sub),
			gocron.WithStartAt(gocron.WithStartDateTime(subStart)),
		)
		if err != nil {
			return err
		}
	}

	clog("all %d jobs registered", 3+len(subreddits))
	return nil
}

func (s *Scheduler) Start() {
	clog("starting")
	s.scheduler.Start()
}

func (s *Scheduler) Stop() error {
	clog("stopping")
	return s.scheduler.Shutdown()
}
