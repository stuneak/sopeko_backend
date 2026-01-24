package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	db "github.com/stuneak/sopeko/db/sqlc"
)

// Excluded usernames (mods, bots, special accounts)
var excludedUsernames = []string{
	"OhWowMuchFunYouGuys",
	"miamihausjunkie",
	"AnnArchist",
	"Immersions-",
	"butthoofer",
	"AutoModerator",
	"PennyBotWeekly",
	"PennyPumper",
	"TransSpeciesDog",
	"the_male_nurse",
	"VisualMod",
	"OPINION_IS_UNPOPULAR",
	"zjz",
	"OSRSkarma",
	"Dan_inKuwait",
	"Swiifttx",
	"teddy_riesling",
	"Stylux",
	"Latter-day_weeb",
	"ShopBitter",
	"CHAINSAW_VASECTOMY",
}

type MentionResponse struct {
	Symbol           string    `json:"symbol"`
	MentionPrice     string    `json:"mention_price"`
	CurrentPrice     string    `json:"current_price"`
	CurrentPriceDate time.Time `json:"current_price_date"`
	PercentChange    string    `json:"percent_change"`
	SplitRatio       float64   `json:"split_ratio"`
	MentionedAt      time.Time `json:"mentioned_at"`
}

func (server *Server) getUserMentions(ctx *gin.Context) {
	username := ctx.Param("username")

	for _, u := range excludedUsernames {
		if u == username {
			ctx.JSON(http.StatusOK, []MentionResponse{})
			return
		}
	}

	cutoffTime := parsePeriodCutoff(ctx.Query("period"))

	mentions, err := server.store.GetUserMentionsComplete(ctx, db.GetUserMentionsCompleteParams{
		Username:    username,
		MentionedAt: cutoffTime,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	results := make([]MentionResponse, 0, len(mentions))
	for _, m := range mentions {
		mentionPrice := fmt.Sprintf("%v", m.MentionPrice)
		currentPrice := fmt.Sprintf("%v", m.CurrentPrice)

		adjustedMentionPrice := adjustPriceForSplits(mentionPrice, m.SplitRatio)

		results = append(results, MentionResponse{
			Symbol:           m.Symbol,
			MentionPrice:     adjustedMentionPrice,
			CurrentPrice:     currentPrice,
			CurrentPriceDate: m.CurrentPriceDate,
			PercentChange:    calculatePercentChange(adjustedMentionPrice, currentPrice),
			SplitRatio:       m.SplitRatio,
			MentionedAt:      m.MentionedAt,
		})
	}

	ctx.JSON(http.StatusOK, results)
}

func calculatePercentChange(oldPrice, newPrice string) string {
	change := calculatePercentChangeFloat(oldPrice, newPrice)
	if change >= 0 {
		return fmt.Sprintf("+%.2f%%", change)
	}
	return fmt.Sprintf("%.2f%%", change)
}

func calculatePercentChangeFloat(oldPrice, newPrice string) float64 {
	old, err := strconv.ParseFloat(oldPrice, 64)
	if err != nil || old == 0 {
		return 0
	}
	newP, err := strconv.ParseFloat(newPrice, 64)
	if err != nil {
		return 0
	}
	return ((newP - old) / old) * 100
}

func adjustPriceForSplits(price string, splitRatio float64) string {
	p, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return price
	}

	adjustedMentionPrice := p
	if splitRatio != 1.0 {
		adjustedMentionPrice = p * splitRatio
	}

	return fmt.Sprintf("%.2f", adjustedMentionPrice)
}

func parsePeriodCutoff(period string) time.Time {
	now := time.Now()
	switch period {
	case "daily":
		return now.AddDate(0, 0, -1)
	case "weekly":
		return now.AddDate(0, 0, -7)
	case "monthly":
		return now.AddDate(0, -1, 0)
	default:
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
}

func (server *Server) getExcludedUsernames(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, excludedUsernames)
}

type PickDetail struct {
	Symbol       string  `json:"symbol"`
	PickPrice    string  `json:"pick_price"`
	CurrentPrice string  `json:"current_price"`
	PercentGain  float64 `json:"percent_gain"`
	SplitRatio   float64 `json:"split_ratio"`
}

type TopUserResponse struct {
	Username         string       `json:"username"`
	TotalPercentGain float64      `json:"total_percent_gain"`
	Picks            []PickDetail `json:"picks"`
}

type PickPerformanceResponse struct {
	Symbol           string    `json:"symbol"`
	MentionPrice     string    `json:"mention_price"`
	CurrentPrice     string    `json:"current_price"`
	CurrentPriceDate time.Time `json:"current_price_date"`
	PercentChange    float64   `json:"percent_change"`
	SplitRatio       float64   `json:"split_ratio"`
	MentionedAt      time.Time `json:"mentioned_at"`
}

func (server *Server) getTopPerformingPicks(ctx *gin.Context) {
	server.getPerformingPicks(ctx, true)
}

func (server *Server) getWorstPerformingPicks(ctx *gin.Context) {
	server.getPerformingPicks(ctx, false)
}

func (server *Server) getPerformingPicks(ctx *gin.Context, topPerformers bool) {
	cutoffTime := parsePeriodCutoff(ctx.Query("period"))

	excludedMap := make(map[string]bool)
	for _, u := range excludedUsernames {
		excludedMap[u] = true
	}

	mentions, err := server.store.GetAllMentionsComplete(ctx, cutoffTime)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	results := make([]PickPerformanceResponse, 0)
	for _, m := range mentions {
		if excludedMap[m.Username] {
			continue
		}

		mentionPrice := fmt.Sprintf("%v", m.MentionPrice)
		currentPrice := fmt.Sprintf("%v", m.CurrentPrice)
		adjustedMentionPrice := adjustPriceForSplits(mentionPrice, m.SplitRatio)
		pctChange := calculatePercentChangeFloat(adjustedMentionPrice, currentPrice)

		results = append(results, PickPerformanceResponse{
			Symbol:           m.Symbol,
			MentionPrice:     adjustedMentionPrice,
			CurrentPrice:     currentPrice,
			CurrentPriceDate: m.CurrentPriceDate,
			PercentChange:    pctChange,
			SplitRatio:       m.SplitRatio,
			MentionedAt:      m.MentionedAt,
		})
	}

	if topPerformers {
		sort.Slice(results, func(i, j int) bool {
			return results[i].PercentChange > results[j].PercentChange
		})
	} else {
		sort.Slice(results, func(i, j int) bool {
			return results[i].PercentChange < results[j].PercentChange
		})
	}

	if len(results) > 50 {
		results = results[:50]
	}

	ctx.JSON(http.StatusOK, results)
}

func (server *Server) getTopPerformingUsers(ctx *gin.Context) {
	cutoffTime := parsePeriodCutoff(ctx.Query("period"))

	excludedMap := make(map[string]bool)
	for _, u := range excludedUsernames {
		excludedMap[u] = true
	}

	mentions, err := server.store.GetAllMentionsComplete(ctx, cutoffTime)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	users := make(map[string]*TopUserResponse)
	for _, m := range mentions {
		if excludedMap[m.Username] {
			continue
		}

		mentionPrice := fmt.Sprintf("%v", m.MentionPrice)
		currentPrice := fmt.Sprintf("%v", m.CurrentPrice)
		adjustedMentionPrice := adjustPriceForSplits(mentionPrice, m.SplitRatio)
		pctChange := calculatePercentChangeFloat(adjustedMentionPrice, currentPrice)

		user, exists := users[m.Username]
		if !exists {
			user = &TopUserResponse{
				Username: m.Username,
				Picks:    []PickDetail{},
			}
			users[m.Username] = user
		}

		user.TotalPercentGain += pctChange
		user.Picks = append(user.Picks, PickDetail{
			Symbol:       m.Symbol,
			PickPrice:    adjustedMentionPrice,
			CurrentPrice: currentPrice,
			PercentGain:  pctChange,
			SplitRatio:   m.SplitRatio,
		})
	}

	results := make([]TopUserResponse, 0, len(users))
	for _, u := range users {
		results = append(results, *u)
	}

	filtered := make([]TopUserResponse, 0, len(results))
	for _, r := range results {
		if r.TotalPercentGain >= 0 {
			filtered = append(filtered, r)
		}
	}
	results = filtered

	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalPercentGain > results[j].TotalPercentGain
	})

	if len(results) > 10 {
		results = results[:10]
	}

	ctx.JSON(http.StatusOK, results)
}
