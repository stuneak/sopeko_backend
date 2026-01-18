package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	redditBaseURL = "https://www.reddit.com"
	userAgent     = "Mozilla/5.0 (compatible; StockMentionBot/1.0)"
)

var rlog = log.New(log.Writer(), "[REDDIT] ", log.Flags())

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
			t := time.Unix(int64(c.Data.CreatedUTC), 0)
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
	seen := make(map[string]bool)
	var comments []RedditComment

	for _, sort := range []string{"confidence", "new", "top"} {
		url := fmt.Sprintf("%s/r/%s/comments/%s.json?limit=500&depth=100&sort=%s",
			redditBaseURL, subreddit, postID, sort)

		body, err := r.makeRequest(ctx, url)
		if err != nil {
			rlog.Printf("Warning: fetch comments sort=%s: %v", sort, err)
			continue
		}

		newComments, moreIDs := r.parseCommentsResponse(body, postID)

		for _, c := range newComments {
			if !seen[c.ID] {
				seen[c.ID] = true
				comments = append(comments, c)
			}
		}

		if len(moreIDs) > 0 {
			rlog.Printf("Post %s: fetching %d more children (sort=%s)", postID, len(moreIDs), sort)
			more, _ := r.fetchMoreChildren(ctx, postID, moreIDs, seen)
			comments = append(comments, more...)
		}

		time.Sleep(1 * time.Second)
	}

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

		rlog.Printf("Post %s: batch %d, fetching %d IDs, %d remaining", postID, i+1, len(batch), len(pending))

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
	rlog.Printf("Starting scrape for r/%s", subreddit)

	posts, err := r.FetchSubredditPosts(ctx, subreddit)
	if err != nil {
		return nil, nil, err
	}

	rlog.Printf("Found %d posts in r/%s", len(posts), subreddit)

	var allComments []RedditComment

	for i, post := range posts {
		if post.NumComments == 0 {
			continue
		}

		comments, err := r.FetchPostComments(ctx, subreddit, post.ID)
		if err != nil {
			rlog.Printf("Warning: post %s: %v", post.ID, err)
			continue
		}

		allComments = append(allComments, comments...)
		rlog.Printf("Post %d/%d (%s): %d comments (expected ~%d)",
			i+1, len(posts), post.ID, len(comments), post.NumComments)

		time.Sleep(2 * time.Second)
	}

	rlog.Printf("Done r/%s: %d posts, %d comments", subreddit, len(posts), len(allComments))
	return posts, allComments, nil
}
