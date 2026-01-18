package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type MentionResponse struct {
	Symbol        string    `json:"symbol"`
	MentionPrice  string    `json:"mention_price"`
	CurrentPrice  string    `json:"current_price"`
	PercentChange string    `json:"percent_change"`
	MentionedAt   time.Time `json:"mentioned_at"`
}

func (server *Server) getUserMentions(ctx *gin.Context) {
	username := ctx.Param("username")

	mentions, err := server.store.GetFirstMentionPerTickerByUsername(ctx, username)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(mentions) == 0 {
		ctx.JSON(http.StatusOK, []MentionResponse{})
		return
	}

	// Exclude today's mentions
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var results []MentionResponse

	for _, mention := range mentions {
		if mention.MentionedAt.After(todayStart) || mention.MentionedAt.Equal(todayStart) {
			continue
		}

		// Fetch current price for each ticker
		latestPrice, err := server.store.GetLatestTickerPrice(ctx, mention.TickerID)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch current price: " + err.Error()})
			return
		}
		currentPrice := latestPrice.Price

		percentChange := calculatePercentChange(mention.MentionPrice, currentPrice)
		results = append(results, MentionResponse{
			Symbol:        mention.Symbol,
			MentionPrice:  mention.MentionPrice,
			CurrentPrice:  currentPrice,
			PercentChange: percentChange,
			MentionedAt:   mention.MentionedAt,
		})
	}

	ctx.JSON(http.StatusOK, results)
}

func calculatePercentChange(oldPrice, newPrice string) string {
	old, err := strconv.ParseFloat(oldPrice, 64)
	if err != nil || old == 0 {
		return "0.00%"
	}
	new, err := strconv.ParseFloat(newPrice, 64)
	if err != nil {
		return "0.00%"
	}

	change := ((new - old) / old) * 100
	if change >= 0 {
		return fmt.Sprintf("+%.2f%%", change)
	}
	return fmt.Sprintf("%.2f%%", change)
}
