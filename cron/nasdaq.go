package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	nasdaqAPIURL  = "https://api.nasdaq.com/api/screener/stocks?tableonly=true&limit=60000&offset=0&download=true"
	nasdaqTimeout = 60 * time.Second
)

type NasdaqFetcher struct {
	client *http.Client
}

type nasdaqResponse struct {
	Data struct {
		Rows []nasdaqStock `json:"rows"`
	} `json:"data"`
}

type nasdaqStock struct {
	Symbol  string `json:"symbol"`
	Name    string `json:"name"`
	Country string `json:"country"`
}

func NewNasdaqFetcher() *NasdaqFetcher {
	return &NasdaqFetcher{
		client: &http.Client{Timeout: nasdaqTimeout},
	}
}

func (n *NasdaqFetcher) FetchStocks(ctx context.Context) ([]nasdaqStock, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", nasdaqAPIURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data nasdaqResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	return data.Data.Rows, nil
}
