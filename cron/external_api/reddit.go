package external_api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var redditLogger = log.New(log.Writer(), "[REDDIT] ", log.Flags())

func rlog(format string, args ...interface{}) {
	pc, _, _, _ := runtime.Caller(1)
	fn := runtime.FuncForPC(pc).Name()
	if i := strings.LastIndex(fn, "."); i >= 0 {
		fn = fn[i+1:]
	}
	redditLogger.Printf(fn+": "+format, args...)
}

const (
	redditBaseURL = "https://www.reddit.com"
	userAgent     = "Mozilla/5.0 (compatible; StockMentionBot/1.0)"
)

var nyLoc = func() *time.Location {
	loc, _ := time.LoadLocation("America/New_York")
	return loc
}()

var skipList = []string{
	"YMMV", "EPS", "TAKE", "MAY", "YOUVE", "ONLY", "LOST", "MONEY", "IF", "YOU",
	"SELL", "YOUR", "USA", "AUS", "UK", "STOCK", "DUE", "FOMO", "SOB", "NO",
	"ETF", "POS", "PENNY", "GTFOH", "NOT", "TOTAL", "DD", "YOLO", "WSB", "RH",
	"FOR", "THE", "MOON", "BUY", "HOLD", "OP", "GATE", "KEEP", "EV", "TRYING",
	"TWICE", "EVERY", "YET", "MOOOON", "THREE", "MEDUSA", "ANNUAL", "MOVERS",
	"VOLUME", "MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "ALASKA",
	"FIRE", "IMHO", "PTSD", "HUGE", "GENIUS", "SCAN", "BAG", "TICKER", "THIS",
	"WEEK", "LETS", "GOOOO", "NASA", "STILL", "OKAY", "RIGHT", "LEMME", "THICC",
	"BEFORE", "GLOBE", "EBITDA", "LMAO", "FAFO", "GET", "LEFT", "BEHIND", "CLASS",
	"VERY", "ADVENT", "HEALTH", "BOYS", "WHICH", "ONE", "JONES", "SODA", "OTC",
	"AND", "RSS", "MARKET", "OF", "SAME", "SUPER", "TOXIC", "ALSO", "NEOW",
	"NASDAQ", "EUR", "USD", "US", "NVIDIA", "IIRC", "ALONE", "WHAT", "SAID",
	"ABOVE", "ADVICE", "DYOR", "ALWAYS", "FOOD", "NYSE", "ISA", "SPX", "BUT",
	"MAYBE", "CALLS", "DRILL", "BABY", "TRUMP", "SHIB", "WILL", "RIP", "EXCEPT",
	"CRYPTO", "DOE", "MIT", "RSI", "DONT", "ENTIRE", "AI", "XYZ", "IS", "LOOK",
	"AT", "IN", "AH", "TODAY", "TIKTOK", "TSA", "CEO", "FDA", "PDUFA", "CTRL",
	"SWOT", "BS", "REAL", "BRUH", "CANADA", "LONG", "LOL", "WAY", "WTF", "PUMP",
	"DUMP", "NEW", "FLAGS", "BOUGHT", "PEAK", "HOLDER", "EOY", "EOW", "IPO",
	"URANUS", "LIGMA", "HELOC", "FY", "LUL", "IT", "PT", "DC", "RS", "LOT",
	"ALT", "PE", "VC", "IBKR", "ATH", "IMO", "NDA", "RELIEF", "COVID", "YTD",
	"MASH", "RUG", "PULL", "PS", "TN", "CUSIP", "FTD", "UCSF", "DO", "IDK",
	"IP", "PR", "IR", "SOME", "GAP", "KEY", "FAST", "DAY", "ANY", "AM", "CALL",
	"PUT", "EXP", "DNN", "MAG", "ARE", "WE", "MA", "MAN", "UP", "DOWN", "AX",
	"LSE", "AMEX", "AMS", "NEVER", "EVER", "COULD", "BE", "NEXT", "IQ", "PB",
	"API", "III", "II", "I", "GL", "ALL", "BIO", "LOW", "BEWARE", "HERE", "INFO",
	"TOUR", "TOP", "BACK", "HOOD", "PD", "PM", "EST", "ICE", "MAGA", "TS", "SCI",
	"DTE", "CC", "NOW", "GO", "EOD", "TACO", "EU", "IRS", "GOOD", "BAD", "LINK",
	"DNA", "CPU", "GPU", "RAM", "SSD", "HDD", "CTO", "ASX", "ARR", "SO",
}

var tickersToSkip = func() map[string]struct{} {
	m := make(map[string]struct{}, len(skipList))
	for _, s := range skipList {
		m[s] = struct{}{}
	}
	return m
}()

// tickerRegex matches uppercase words (2-7 chars) that could be tickers.
// Also matches $TICKER format.
var tickerRegex = regexp.MustCompile(`\$?([A-Z]{2,7})\b`)

// ExtractTickers extracts potential ticker symbols from text,
// filtering out common English words and abbreviations.
func ExtractTickers(text string) []string {
	matches := tickerRegex.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var tickers []string

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		ticker := strings.ToUpper(match[1])
		_, skip := tickersToSkip[ticker]
		if seen[ticker] || skip {
			continue
		}
		seen[ticker] = true
		tickers = append(tickers, ticker)
	}

	return tickers
}

type RedditScraper struct {
	client *http.Client
}

type RedditPost struct {
	ID          string
	Title       string
	Author      string
	Selftext    string
	CreatedAt   time.Time
	URL         string
	Subreddit   string
	NumComments int
}

type RedditComment struct {
	ID        string
	Author    string
	Body      string
	CreatedAt time.Time
	PostID    string
	ParentID  string
}

type redditPostData struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Selftext    string  `json:"selftext"`
	CreatedUTC  float64 `json:"created_utc"`
	Permalink   string  `json:"permalink"`
	Subreddit   string  `json:"subreddit"`
	NumComments int     `json:"num_comments"`
}

type redditListingResponse struct {
	Data struct {
		Children []struct {
			Data redditPostData `json:"data"`
		} `json:"children"`
		After string `json:"after"`
	} `json:"data"`
}

type redditCommentData struct {
	ID         string          `json:"id"`
	Author     string          `json:"author"`
	Body       string          `json:"body"`
	CreatedUTC float64         `json:"created_utc"`
	ParentID   string          `json:"parent_id"`
	Children   []string        `json:"children"`
	Replies    json.RawMessage `json:"replies"`
}

type redditCommentItem struct {
	Kind string            `json:"kind"`
	Data redditCommentData `json:"data"`
}

type redditCommentsResponse []struct {
	Data struct {
		Children []json.RawMessage `json:"children"`
	} `json:"data"`
}

type redditMoreChildrenResponse struct {
	JSON struct {
		Data struct {
			Things []redditCommentItem `json:"things"`
		} `json:"data"`
	} `json:"json"`
}

func NewRedditScraper() *RedditScraper {
	return &RedditScraper{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (r *RedditScraper) makeRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited (429)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (r *RedditScraper) requestWithRetry(ctx context.Context, url string, retries int) ([]byte, error) {
	var lastErr error
	for range retries {
		body, err := r.makeRequest(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		time.Sleep(3 * time.Second)
	}
	return nil, lastErr
}

func (r *RedditScraper) FetchSubredditPosts(ctx context.Context, subreddit string) ([]RedditPost, error) {
	var posts []RedditPost
	cutoff := time.Now().Add(-24 * time.Hour)
	after := ""

	for {
		url := fmt.Sprintf("%s/r/%s/new.json?limit=100", redditBaseURL, subreddit)
		if after != "" {
			url += "&after=" + after
		}

		body, err := r.makeRequest(ctx, url)
		if err != nil {
			return nil, err
		}

		var resp redditListingResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}

		done := false
		for _, c := range resp.Data.Children {
			t := time.Unix(int64(c.Data.CreatedUTC), 0).In(nyLoc)
			if t.Before(cutoff) {
				done = true
				break
			}
			posts = append(posts, RedditPost{
				ID:          c.Data.ID,
				Title:       c.Data.Title,
				Author:      c.Data.Author,
				Selftext:    c.Data.Selftext,
				CreatedAt:   t,
				URL:         redditBaseURL + c.Data.Permalink,
				Subreddit:   c.Data.Subreddit,
				NumComments: c.Data.NumComments,
			})
		}

		if done || resp.Data.After == "" {
			break
		}
		after = resp.Data.After
		time.Sleep(3 * time.Second)
	}

	return posts, nil
}

func (r *RedditScraper) FetchPostComments(ctx context.Context, subreddit, postID string) ([]RedditComment, error) {
	rlog("fetching comments subreddit=%s postID=%s", subreddit, postID)

	seen := make(map[string]bool)
	var comments []RedditComment

	for _, sort := range []string{"new"} {
		url := fmt.Sprintf("%s/r/%s/comments/%s.json?limit=500&depth=100&sort=%s",
			redditBaseURL, subreddit, postID, sort)

		body, err := r.makeRequest(ctx, url)
		if err != nil {
			rlog("request failed for postID=%s sort=%s: %v", postID, sort, err)
			continue
		}

		newComments, moreIDs := r.parseCommentsResponse(body, postID)
		rlog("parsed %d comments, %d moreIDs for postID=%s sort=%s", len(newComments), len(moreIDs), postID, sort)

		for _, c := range newComments {
			if !seen[c.ID] {
				seen[c.ID] = true
				comments = append(comments, c)
			}
		}

		if len(moreIDs) > 0 {
			more, _ := r.fetchMoreChildren(ctx, postID, moreIDs, seen)
			rlog("fetched %d more children for postID=%s", len(more), postID)
			comments = append(comments, more...)
		}

		time.Sleep(1 * time.Second)
	}

	rlog("done postID=%s total_comments=%d", postID, len(comments))
	return comments, nil
}

func (r *RedditScraper) parseCommentsResponse(body []byte, postID string) ([]RedditComment, []string) {
	var resp redditCommentsResponse
	if err := json.Unmarshal(body, &resp); err != nil || len(resp) < 2 {
		return nil, nil
	}

	return r.parseChildren(resp[1].Data.Children, postID)
}

func (r *RedditScraper) parseChildren(children []json.RawMessage, postID string) ([]RedditComment, []string) {
	var comments []RedditComment
	var moreIDs []string

	for _, raw := range children {
		var item redditCommentItem
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}

		if item.Kind == "more" {
			moreIDs = append(moreIDs, item.Data.Children...)
			continue
		}

		if item.Kind != "t1" {
			continue
		}

		comments = append(comments, RedditComment{
			ID:        item.Data.ID,
			Author:    item.Data.Author,
			Body:      item.Data.Body,
			CreatedAt: time.Unix(int64(item.Data.CreatedUTC), 0),
			PostID:    postID,
			ParentID:  item.Data.ParentID,
		})

		if len(item.Data.Replies) > 0 && string(item.Data.Replies) != `""` {
			var replies struct {
				Data struct {
					Children []json.RawMessage `json:"children"`
				} `json:"data"`
			}
			if json.Unmarshal(item.Data.Replies, &replies) == nil {
				nested, nestedMore := r.parseChildren(replies.Data.Children, postID)
				comments = append(comments, nested...)
				moreIDs = append(moreIDs, nestedMore...)
			}
		}
	}

	return comments, moreIDs
}

func (r *RedditScraper) fetchMoreChildren(ctx context.Context, postID string, ids []string, seen map[string]bool) ([]RedditComment, error) {
	pending := make(map[string]bool)
	for _, id := range ids {
		if !seen[id] {
			pending[id] = true
		}
	}

	var comments []RedditComment

	for i := 0; len(pending) > 0 && i < 50; i++ {
		batch := make([]string, 0, 100)
		for id := range pending {
			batch = append(batch, id)
			delete(pending, id)
			if len(batch) >= 100 {
				break
			}
		}

		url := fmt.Sprintf("%s/api/morechildren.json?api_type=json&link_id=t3_%s&children=%s&limit_children=false",
			redditBaseURL, postID, strings.Join(batch, ","))

		body, err := r.requestWithRetry(ctx, url, 3)
		if err != nil {
			continue
		}

		var resp redditMoreChildrenResponse
		if json.Unmarshal(body, &resp) != nil {
			continue
		}

		for _, thing := range resp.JSON.Data.Things {
			if thing.Kind == "more" {
				for _, id := range thing.Data.Children {
					if !seen[id] && !pending[id] {
						pending[id] = true
					}
				}
			} else if thing.Kind == "t1" && !seen[thing.Data.ID] {
				seen[thing.Data.ID] = true
				comments = append(comments, RedditComment{
					ID:        thing.Data.ID,
					Author:    thing.Data.Author,
					Body:      thing.Data.Body,
					CreatedAt: time.Unix(int64(thing.Data.CreatedUTC), 0),
					PostID:    postID,
					ParentID:  thing.Data.ParentID,
				})
			}
		}

		if len(pending) > 0 {
			time.Sleep(2 * time.Second)
		}
	}

	return comments, nil
}

func (r *RedditScraper) ScrapeSubreddit(ctx context.Context, subreddit string) ([]RedditPost, []RedditComment, error) {
	posts, err := r.FetchSubredditPosts(ctx, subreddit)
	if err != nil {
		return nil, nil, err
	}

	var allComments []RedditComment

	for _, post := range posts {
		if post.NumComments == 0 {
			continue
		}

		comments, err := r.FetchPostComments(ctx, subreddit, post.ID)
		if err != nil {
			continue
		}

		allComments = append(allComments, comments...)
		time.Sleep(2 * time.Second)
	}

	return posts, allComments, nil
}
