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

	mentions, err := server.store.GetLastTwoMentionsByUsername(ctx, username)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(mentions) == 0 {
		ctx.JSON(http.StatusOK, []MentionResponse{})
		return
	}

	// Check if the last mention was today
	now := time.Now()
	lastMention := mentions[0]
	if lastMention.MentionedAt.Year() == now.Year() &&
		lastMention.MentionedAt.YearDay() == now.YearDay() {
		ctx.JSON(http.StatusOK, []MentionResponse{})
		return
	}

	// Fetch current price from ticker_prices table
	latestPrice, err := server.store.GetLatestTickerPrice(ctx, lastMention.TickerID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch current price: " + err.Error()})
		return
	}
	currentPrice := latestPrice.Price

	var response []MentionResponse

	// If we have 2 mentions, include the previous one first
	if len(mentions) >= 2 {
		prevMention := mentions[1]
		prevPercentChange := calculatePercentChange(prevMention.MentionPrice, currentPrice)
		response = append(response, MentionResponse{
			Symbol:        prevMention.Symbol,
			MentionPrice:  prevMention.MentionPrice,
			CurrentPrice:  currentPrice,
			PercentChange: prevPercentChange,
			MentionedAt:   prevMention.MentionedAt,
		})
	}

	// Last mention (first in the list)
	lastPercentChange := calculatePercentChange(lastMention.MentionPrice, currentPrice)
	response = append(response, MentionResponse{
		Symbol:        lastMention.Symbol,
		MentionPrice:  lastMention.MentionPrice,
		CurrentPrice:  currentPrice,
		PercentChange: lastPercentChange,
		MentionedAt:   lastMention.MentionedAt,
	})

	ctx.JSON(http.StatusOK, response)
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
