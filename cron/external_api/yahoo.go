package external_api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type YahooFetcher struct {
	client *http.Client
}

type SplitEvent struct {
	Ratio         float64
	EffectiveDate time.Time
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice  float64 `json:"regularMarketPrice"`
				RegularMarketVolume int64   `json:"regularMarketVolume"`
				RegularMarketTime   int64   `json:"regularMarketTime"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close  []*float64 `json:"close"`
					Volume []*int64   `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
			Events *struct {
				Splits map[string]yahooSplitEvent `json:"splits"`
			} `json:"events"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type yahooSplitEvent struct {
	Date        int64   `json:"date"`
	Numerator   float64 `json:"numerator"`
	Denominator float64 `json:"denominator"`
}

func NewYahooFetcher() *YahooFetcher {
	return &YahooFetcher{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchCurrentPriceAndVolume fetches the previous day's closing price, volume, and timestamp for a symbol.
func (y *YahooFetcher) FetchCurrentPriceAndVolume(ctx context.Context, symbol string) (price float64, volume int64, recordedAt time.Time, err error) {
	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?range=5d&interval=1d",
		symbol,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; StockMentionBot/1.0)")

	resp, err := y.client.Do(req)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, time.Time{}, fmt.Errorf("yahoo finance returned status %d for %s", resp.StatusCode, symbol)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, time.Time{}, err
	}

	var chartResp yahooChartResponse
	if err := json.Unmarshal(body, &chartResp); err != nil {
		return 0, 0, time.Time{}, err
	}

	if chartResp.Chart.Error != nil {
		return 0, 0, time.Time{}, fmt.Errorf("yahoo API error for %s: %s", symbol, chartResp.Chart.Error.Description)
	}

	if len(chartResp.Chart.Result) == 0 {
		return 0, 0, time.Time{}, fmt.Errorf("no chart data for %s", symbol)
	}

	result := chartResp.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 || len(result.Timestamp) < 2 {
		return 0, 0, time.Time{}, fmt.Errorf("insufficient chart data for %s", symbol)
	}

	quotes := result.Indicators.Quote[0]
	// Get the second-to-last entry (previous day's close)
	idx := len(result.Timestamp) - 2
	if idx < 0 || idx >= len(quotes.Close) {
		return 0, 0, time.Time{}, fmt.Errorf("insufficient price data for %s", symbol)
	}

	if quotes.Close[idx] == nil {
		return 0, 0, time.Time{}, fmt.Errorf("no closing price for previous day for %s", symbol)
	}

	closePrice := *quotes.Close[idx]
	var vol int64
	if idx < len(quotes.Volume) && quotes.Volume[idx] != nil {
		vol = *quotes.Volume[idx]
	}

	return closePrice, vol, time.Unix(result.Timestamp[idx], 0), nil
}

// FetchSplits fetches all stock split events for a symbol.
func (y *YahooFetcher) FetchSplits(ctx context.Context, symbol string) ([]SplitEvent, error) {
	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?range=max&interval=1d&events=splits",
		symbol,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; StockMentionBot/1.0)")

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo finance returned status %d for %s", resp.StatusCode, symbol)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var chartResp yahooChartResponse
	if err := json.Unmarshal(body, &chartResp); err != nil {
		return nil, err
	}

	if chartResp.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo API error for %s: %s", symbol, chartResp.Chart.Error.Description)
	}

	if len(chartResp.Chart.Result) == 0 {
		return nil, nil
	}

	result := chartResp.Chart.Result[0]
	if result.Events == nil || len(result.Events.Splits) == 0 {
		return nil, nil
	}

	var splits []SplitEvent
	for _, split := range result.Events.Splits {
		if split.Numerator == 0 {
			continue
		}
		ratio := split.Denominator / split.Numerator
		splits = append(splits, SplitEvent{
			Ratio:         ratio,
			EffectiveDate: time.Unix(split.Date, 0),
		})
	}

	return splits, nil
}
