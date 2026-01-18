package cron

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	db "github.com/stuneak/bagger/db/sqlc"
)

var plog = log.New(log.Writer(), "[PARSER] ", log.Flags())

var tickersToSkip = map[string]bool{
	"YMMV": true, "EPS": true, "TAKE": true, "MAY": true, "YOUVE": true,
	"ONLY": true, "LOST": true, "MONEY": true, "IF": true, "YOU": true,
	"SELL": true, "YOUR": true, "USA": true, "AUS": true, "UK": true,
	"STOCK": true, "DUE": true, "FOMO": true, "SOB": true, "NO": true,
	"ETF": true, "POS": true, "PENNY": true, "GTFOH": true, "NOT": true,
	"TOTAL": true, "DD": true, "YOLO": true, "WSB": true, "RH": true,
	"FOR": true, "THE": true, "MOON": true, "BUY": true, "HOLD": true,
	"OP": true, "GATE": true, "KEEP": true, "EV": true, "TRYING": true,
	"TWICE": true, "EVERY": true, "YET": true, "MOOOON": true, "THREE": true,
	"MEDUSA": true, "ANNUAL": true, "MOVERS": true, "VOLUME": true,
	"MONDAY": true, "TUESDAY": true, "WEDNESDAY": true, "THURSDAY": true,
	"FRIDAY": true, "ALASKA": true, "FIRE": true, "IMHO": true, "PTSD": true,
	"HUGE": true, "GENIUS": true, "SCAN": true, "BAG": true, "TICKER": true,
	"THIS": true, "WEEK": true, "LETS": true, "GOOOO": true, "NASA": true,
	"STILL": true, "OKAY": true, "RIGHT": true, "LEMME": true, "THICC": true,
	"BEFORE": true, "GLOBE": true, "EBITDA": true, "LMAO": true, "FAFO": true,
	"GET": true, "LEFT": true, "BEHIND": true, "CLASS": true, "VERY": true,
	"ADVENT": true, "HEALTH": true, "BOYS": true, "WHICH": true, "ONE": true,
	"JONES": true, "SODA": true, "OTC": true, "AND": true, "RSS": true,
	"MARKET": true, "OF": true, "SAME": true, "SUPER": true, "TOXIC": true,
	"ALSO": true, "NEOW": true, "NASDAQ": true, "EUR": true, "USD": true,
	"US": true, "NVIDIA": true, "IIRC": true, "ALONE": true, "WHAT": true,
	"SAID": true, "ABOVE": true, "ADVICE": true, "DYOR": true, "ALWAYS": true,
	"FOOD": true, "NYSE": true, "ISA": true, "SPX": true, "BUT": true,
	"MAYBE": true, "CALLS": true, "DRILL": true, "BABY": true, "TRUMP": true,
	"SHIB": true, "WILL": true, "RIP": true, "EXCEPT": true, "CRYPTO": true,
	"DOE": true, "MIT": true, "RSI": true, "DONT": true, "ENTIRE": true,
	"AI": true, "XYZ": true, "IS": true, "LOOK": true, "AT": true, "IN": true,
	"AH": true, "TODAY": true, "TIKTOK": true, "TSA": true, "CEO": true,
	"FDA": true, "PDUFA": true, "CTRL": true, "SWOT": true, "BS": true,
	"REAL": true, "BRUH": true, "CANADA": true, "LONG": true, "LOL": true,
	"WAY": true, "WTF": true, "PUMP": true, "DUMP": true, "NEW": true,
	"FLAGS": true, "BOUGHT": true, "PEAK": true, "HOLDER": true, "EOY": true,
	"EOW": true, "IPO": true, "URANUS": true, "LIGMA": true, "HELOC": true,
	"FY": true, "LUL": true, "IT": true, "PT": true, "DC": true, "RS": true,
	"LOT": true, "ALT": true, "PE": true, "VC": true, "IBKR": true,
	"ATH": true, "IMO": true, "NDA": true, "RELIEF": true, "COVID": true,
	"YTD": true, "MASH": true, "RUG": true, "PULL": true, "PS": true,
	"TN": true, "CUSIP": true, "FTD": true, "UCSF": true, "DO": true,
	"IDK": true, "IP": true, "PR": true, "IR": true, "SOME": true,
	"GAP": true, "KEY": true, "FAST": true, "DAY": true, "ANY": true,
	"AM": true, "CALL": true, "PUT": true, "EXP": true, "DNN": true,
	"MAG": true, "ARE": true, "WE": true, "MA": true, "MAN": true,
	"UP": true, "DOWN": true, "AX": true, "LSE": true, "AMEX": true,
	"AMS": true, "NEVER": true, "EVER": true, "COULD": true, "BE": true,
	"NEXT": true, "IQ": true, "PB": true, "API": true, "III": true,
	"II": true, "I": true, "GL": true, "ALL": true, "BIO": true,
	"LOW": true, "BEWARE": true, "HERE": true, "INFO": true, "TOUR": true,
	"TOP": true, "BACK": true, "HOOD": true, "PD": true, "PM": true,
	"EST": true, "ICE": true, "MAGA": true, "TS": true, "SCI": true,
	"DTE": true, "CC": true, "NOW": true, "GO": true, "EOD": true,
}

// tickerRegex matches uppercase words (2-7 chars) that could be tickers
// Also matches $TICKER format
var tickerRegex = regexp.MustCompile(`\$?([A-Z]{2,7})\b`)

type TickerParser struct {
	store  *db.Queries
	client *http.Client
}

func NewTickerParser(store *db.Queries) *TickerParser {
	return &TickerParser{
		store:  store,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// ExtractTickers extracts potential ticker symbols from text
func (p *TickerParser) ExtractTickers(text string) []string {
	matches := tickerRegex.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var tickers []string

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		ticker := match[1] // Get the captured group (without $)
		ticker = strings.ToUpper(ticker)

		// Skip if already seen or in skip list
		if seen[ticker] || tickersToSkip[ticker] {
			continue
		}
		seen[ticker] = true
		tickers = append(tickers, ticker)
	}

	return tickers
}

// ProcessPosts processes Reddit posts for ticker mentions
func (p *TickerParser) ProcessPosts(ctx context.Context, posts []RedditPost) error {
	plog.Printf("Processing %d posts for ticker mentions", len(posts))
	for i, post := range posts {
		if i > 0 && i%10 == 0 {
			plog.Printf("Processed %d/%d posts", i, len(posts))
		}
		// Combine title and selftext for ticker extraction
		text := post.Title + " " + post.Selftext
		tickers := p.ExtractTickers(text)

		if len(tickers) == 0 {
			continue
		}

		// Filter tickers to only those that exist in DB
		validTickers := p.filterExistingTickers(ctx, tickers)
		if len(validTickers) == 0 {
			continue
		}

		// Get or create user
		user, err := p.getOrCreateUser(ctx, post.Author)
		if err != nil {
			plog.Printf("Error getting/creating user %s: %v", post.Author, err)
			continue
		}

		// Create or get existing comment for the post
		comment, err := p.store.CreateComment(ctx, db.CreateCommentParams{
			UserID:     user.ID,
			Source:     "reddit",
			ExternalID: "t3_" + post.ID,
			Content:    text,
			CreatedAt:  post.CreatedAt,
		})
		if err != nil {
			plog.Printf("Error creating comment for post %s: %v", post.ID, err)
			continue
		}

		// Process each valid ticker mention
		for _, ticker := range validTickers {
			if err := p.processTicker(ctx, ticker, user.ID, comment.ID, post.CreatedAt); err != nil {
				plog.Printf("Error processing ticker %s: %v", ticker.Symbol, err)
			}
		}
	}

	plog.Printf("Finished processing %d posts", len(posts))
	return nil
}

// ProcessComments processes Reddit comments for ticker mentions
func (p *TickerParser) ProcessComments(ctx context.Context, comments []RedditComment) error {
	plog.Printf("Processing %d comments for ticker mentions", len(comments))
	for i, redditComment := range comments {
		if i > 0 && i%100 == 0 {
			plog.Printf("Processed %d/%d comments", i, len(comments))
		}
		tickers := p.ExtractTickers(redditComment.Body)

		if len(tickers) == 0 {
			continue
		}

		// Filter tickers to only those that exist in DB
		validTickers := p.filterExistingTickers(ctx, tickers)
		if len(validTickers) == 0 {
			continue
		}

		// Get or create user
		user, err := p.getOrCreateUser(ctx, redditComment.Author)
		if err != nil {
			plog.Printf("Error getting/creating user %s: %v", redditComment.Author, err)
			continue
		}

		// Create or get existing comment
		comment, err := p.store.CreateComment(ctx, db.CreateCommentParams{
			UserID:     user.ID,
			Source:     "reddit",
			ExternalID: "t1_" + redditComment.ID,
			Content:    redditComment.Body,
			CreatedAt:  redditComment.CreatedAt,
		})
		if err != nil {
			plog.Printf("Error creating comment %s: %v", redditComment.ID, err)
			continue
		}

		// Process each valid ticker mention
		for _, ticker := range validTickers {
			if err := p.processTicker(ctx, ticker, user.ID, comment.ID, redditComment.CreatedAt); err != nil {
				plog.Printf("Error processing ticker %s: %v", ticker.Symbol, err)
			}
		}
	}

	plog.Printf("Finished processing %d comments", len(comments))
	return nil
}

func (p *TickerParser) getOrCreateUser(ctx context.Context, username string) (db.User, error) {
	// Try to get existing user
	user, err := p.store.GetUserByUsername(ctx, username)
	if err == nil {
		return user, nil
	}

	if err != sql.ErrNoRows {
		return db.User{}, err
	}

	// Create new user
	return p.store.CreateUser(ctx, username)
}

// filterExistingTickers filters ticker symbols to only those that exist in the database
func (p *TickerParser) filterExistingTickers(ctx context.Context, symbols []string) []db.Ticker {
	var validTickers []db.Ticker
	for _, symbol := range symbols {
		ticker, err := p.store.GetTickerBySymbol(ctx, symbol)
		if err != nil {
			if err != sql.ErrNoRows {
				plog.Printf("Error checking ticker %s: %v", symbol, err)
			}
			continue
		}
		validTickers = append(validTickers, ticker)
	}
	return validTickers
}

func (p *TickerParser) processTicker(ctx context.Context, ticker db.Ticker, userID, commentID int64, mentionedAt time.Time) error {
	var price string
	var priceID int64

	// Check if price already exists for this ticker on this date
	existingPrice, err := p.store.GetTickerPriceByDate(ctx, db.GetTickerPriceByDateParams{
		TickerID: ticker.ID,
		Date:     mentionedAt,
	})
	if err == nil {
		// Price exists, use it
		price = existingPrice.Price
		priceID = existingPrice.ID
		plog.Printf("Using existing price for %s on %v: %s", ticker.Symbol, mentionedAt.Format("2006-01-02"), price)
	} else if err == sql.ErrNoRows {
		// No price exists, fetch from Yahoo
		fetchedPrice, priceTime, fetchErr := p.fetchHistoricalPrice(ctx, ticker.Symbol, mentionedAt)
		if fetchErr != nil {
			plog.Printf("Error fetching price for %s at %v: %v", ticker.Symbol, mentionedAt, fetchErr)
			fetchedPrice = "0"
			priceTime = mentionedAt
		}

		// Insert price into ticker_prices
		insertedPrice, insertErr := p.store.InsertTickerPrice(ctx, db.InsertTickerPriceParams{
			TickerID:   ticker.ID,
			Price:      fetchedPrice,
			RecordedAt: priceTime,
		})
		if insertErr != nil {
			plog.Printf("Error inserting ticker price: %v", insertErr)
		} else {
			price = insertedPrice.Price
			priceID = insertedPrice.ID
		}
	} else {
		plog.Printf("Error checking existing price for %s: %v", ticker.Symbol, err)
	}

	// Create ticker mention
	_, err = p.store.CreateTickerMention(ctx, db.CreateTickerMentionParams{
		TickerID:    ticker.ID,
		UserID:      userID,
		CommentID:   commentID,
		MentionedAt: mentionedAt,
		PriceID:     priceID,
	})
	if err != nil {
		return fmt.Errorf("error creating ticker mention: %w", err)
	}

	plog.Printf("Created mention: %s by user %d at price %s", ticker.Symbol, userID, price)
	return nil
}

// Yahoo Finance API response structures
type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// fetchHistoricalPrice fetches the price at or before the given time
func (p *TickerParser) fetchHistoricalPrice(ctx context.Context, symbol string, targetTime time.Time) (string, time.Time, error) {
	// Calculate time range: from 7 days before to target time
	startTime := targetTime.Add(-7 * 24 * time.Hour).Unix()
	endTime := targetTime.Unix()

	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d",
		symbol, startTime, endTime,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; StockMentionBot/1.0)")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("yahoo finance returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, err
	}

	var chartResp yahooChartResponse
	if err := json.Unmarshal(body, &chartResp); err != nil {
		return "", time.Time{}, err
	}

	if chartResp.Chart.Error != nil {
		return "", time.Time{}, fmt.Errorf("yahoo API error: %s", chartResp.Chart.Error.Description)
	}

	if len(chartResp.Chart.Result) == 0 {
		return "", time.Time{}, fmt.Errorf("no data returned for %s", symbol)
	}

	result := chartResp.Chart.Result[0]
	if len(result.Timestamp) == 0 || len(result.Indicators.Quote) == 0 || len(result.Indicators.Quote[0].Close) == 0 {
		return "", time.Time{}, fmt.Errorf("no price data for %s", symbol)
	}

	// Find the closest price at or before target time
	timestamps := result.Timestamp
	closes := result.Indicators.Quote[0].Close

	targetUnix := targetTime.Unix()

	// Find latest valid price at or before target time
	bestIdx := findLatestPriceBeforeTarget(timestamps, closes, targetUnix)

	// Fallback to earliest available price
	if bestIdx == -1 {
		bestIdx = findFirstValidPrice(closes)
	}

	if bestIdx == -1 {
		return "", time.Time{}, fmt.Errorf("no valid price found for %s", symbol)
	}

	priceTime := time.Unix(timestamps[bestIdx], 0)
	price := fmt.Sprintf("%.6f", closes[bestIdx])

	return price, priceTime, nil
}

// findLatestPriceBeforeTarget returns the index of the latest price at or before targetUnix.
// Returns -1 if no valid price is found.
func findLatestPriceBeforeTarget(timestamps []int64, closes []float64, targetUnix int64) int {
	bestIdx := -1
	for i, ts := range timestamps {
		if ts <= targetUnix && i < len(closes) && closes[i] != 0 {
			bestIdx = i
		}
	}
	return bestIdx
}

// findFirstValidPrice returns the index of the first non-zero price.
// Returns -1 if no valid price is found.
func findFirstValidPrice(closes []float64) int {
	for i, price := range closes {
		if price != 0 {
			return i
		}
	}
	return -1
}

// FetchCurrentPrice fetches the current/latest price for a symbol from Yahoo Finance
func (p *TickerParser) FetchCurrentPrice(ctx context.Context, symbol string) (string, error) {
	// Use 1d interval with range of 1d to get latest price
	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d",
		symbol,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; StockMentionBot/1.0)")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("yahoo finance returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var chartResp yahooChartResponse
	if err := json.Unmarshal(body, &chartResp); err != nil {
		return "", err
	}

	if chartResp.Chart.Error != nil {
		return "", fmt.Errorf("yahoo API error: %s", chartResp.Chart.Error.Description)
	}

	if len(chartResp.Chart.Result) == 0 {
		return "", fmt.Errorf("no data returned for %s", symbol)
	}

	result := chartResp.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 || len(result.Indicators.Quote[0].Close) == 0 {
		return "", fmt.Errorf("no price data for %s", symbol)
	}

	// Get the latest close price
	closes := result.Indicators.Quote[0].Close
	for i := len(closes) - 1; i >= 0; i-- {
		if closes[i] != 0 {
			return fmt.Sprintf("%.6f", closes[i]), nil
		}
	}

	return "", fmt.Errorf("no valid price found for %s", symbol)
}
