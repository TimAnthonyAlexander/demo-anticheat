package stats

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

//go:embed report.tmpl.html
var htmlTemplateSource string

// HTMLReporter renders a self-contained HTML report.
type HTMLReporter struct {
	tmpl *template.Template
}

// NewHTMLReporter creates a new HTMLReporter.
func NewHTMLReporter() (*HTMLReporter, error) {
	tmpl, err := template.New("report").Parse(htmlTemplateSource)
	if err != nil {
		return nil, fmt.Errorf("parse html template: %w", err)
	}
	return &HTMLReporter{tmpl: tmpl}, nil
}

// Report writes an HTML report. The categories argument is accepted for
// Reporter compatibility but the HTML reporter derives its own ordering.
func (hr *HTMLReporter) Report(demoStats *DemoStats, _ []Category, writer io.Writer) error {
	data := buildHTMLData(demoStats)
	return hr.tmpl.Execute(writer, data)
}

const (
	flagThreshold    = 50.0
	amberThreshold   = 25.0
	gaugeRadius      = 34.0
	gaugeCircumf     = 2.0 * math.Pi * gaugeRadius
	placeholderSteam = 0
)

type htmlData struct {
	DemoName          string
	MapName           string
	GeneratedAt       string
	PlayerCount       int
	FlaggedCount      int
	HighestLikelihood float64
	HighestName       string
	HighestClass      string
	LowestLikelihood  float64
	LowestName        string
	Spread            float64
	GameMode          string
	RoundCount        int64
	MetricCount       int
	Players           []htmlPlayer
}

type htmlPlayer struct {
	Name            string
	SteamID         string
	Likelihood      float64
	LikelihoodClass string
	Flagged         bool
	Circumference   float64
	DashOffset      float64
	Categories      []htmlCategory
}

type htmlCategory struct {
	Title   string
	Note    string
	Metrics []htmlMetric
}

type htmlMetric struct {
	Label string
	Value string
	Class string
}

func buildHTMLData(ds *DemoStats) htmlData {
	data := htmlData{
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05 MST"),
		DemoName:    fallback(ds.DemoName, "CS2 Demo"),
		MapName:     ds.MapName,
	}

	if global, ok := ds.Players[placeholderSteam]; ok {
		if m, found := global.GetMetric(Category("game_info"), Key("game_mode")); found {
			data.GameMode = m.StringValue
		}
		if m, found := global.GetMetric(Category("game_info"), Key("round_count")); found {
			data.RoundCount = m.IntValue
		}
	}

	realPlayers := make([]*PlayerStats, 0, len(ds.Players))
	for sid, ps := range ds.Players {
		if sid == placeholderSteam {
			continue
		}
		realPlayers = append(realPlayers, ps)
	}

	sort.Slice(realPlayers, func(i, j int) bool {
		li := getMetricFloatValue(realPlayers[i], Category("anti_cheat"), Key("cheat_likelihood"))
		lj := getMetricFloatValue(realPlayers[j], Category("anti_cheat"), Key("cheat_likelihood"))
		if li != lj {
			return li > lj
		}
		return realPlayers[i].Player.Name < realPlayers[j].Player.Name
	})

	data.PlayerCount = len(realPlayers)
	metricCount := 0
	highest := math.Inf(-1)
	lowest := math.Inf(1)

	for _, ps := range realPlayers {
		hp := buildPlayer(ps)
		if hp.Flagged {
			data.FlaggedCount++
		}
		if hp.Likelihood > highest {
			highest = hp.Likelihood
			data.HighestName = ps.Player.Name
		}
		if hp.Likelihood < lowest {
			lowest = hp.Likelihood
			data.LowestName = ps.Player.Name
		}
		for _, cat := range hp.Categories {
			metricCount += len(cat.Metrics)
		}
		data.Players = append(data.Players, hp)
	}

	if data.PlayerCount > 0 {
		data.HighestLikelihood = highest
		data.LowestLikelihood = lowest
		data.Spread = highest - lowest
		data.HighestClass = likelihoodClass(highest)
	}
	data.MetricCount = metricCount

	return data
}

func buildPlayer(ps *PlayerStats) htmlPlayer {
	likelihood := getMetricFloatValue(ps, Category("anti_cheat"), Key("cheat_likelihood"))
	flagged := false
	if m, found := ps.GetMetric(Category("anti_cheat"), Key("cheater")); found && m.StringValue == "Yes" {
		flagged = true
	}

	dashOffset := gaugeCircumf * (1.0 - clamp01(likelihood/100.0))

	return htmlPlayer{
		Name:            fallback(ps.Player.Name, "Unknown"),
		SteamID:         fmt.Sprintf("%d", ps.Player.SteamID64),
		Likelihood:      likelihood,
		LikelihoodClass: likelihoodClass(likelihood),
		Flagged:         flagged,
		Circumference:   gaugeCircumf,
		DashOffset:      dashOffset,
		Categories:      buildCategories(ps),
	}
}

// categoryDisplay defines render order and a friendly title for each known
// category. Unknown categories get appended alphabetically with a title-cased
// label.
var categoryDisplay = []struct {
	Key   Category
	Title string
	Note  string
}{
	{Category("anti_cheat"), "Score Breakdown", ""},
	{Category("kills"), "Combat", ""},
	{Category("aiming"), "Aim Snap", ""},
	{Category("reaction"), "Reaction Time", ""},
	{Category("recoil"), "Recoil Control", ""},
	{Category("weapons"), "Weapon Usage", ""},
	{Category("behavioral"), "Behavioral", "informational"},
	{Category("game_info"), "Game Info", ""},
}

func buildCategories(ps *PlayerStats) []htmlCategory {
	out := make([]htmlCategory, 0, len(categoryDisplay))
	seen := make(map[Category]bool)

	for _, spec := range categoryDisplay {
		seen[spec.Key] = true
		metrics := metricsForCategory(ps, spec.Key)
		if len(metrics) == 0 {
			continue
		}
		out = append(out, htmlCategory{Title: spec.Title, Note: spec.Note, Metrics: metrics})
	}

	leftover := make([]Category, 0)
	for cat := range ps.Categories {
		if !seen[cat] {
			leftover = append(leftover, cat)
		}
	}
	sort.Slice(leftover, func(i, j int) bool { return string(leftover[i]) < string(leftover[j]) })
	for _, cat := range leftover {
		metrics := metricsForCategory(ps, cat)
		if len(metrics) == 0 {
			continue
		}
		out = append(out, htmlCategory{Title: titleize(string(cat)), Metrics: metrics})
	}
	return out
}

func metricsForCategory(ps *PlayerStats, cat Category) []htmlMetric {
	keys := make([]Key, 0)
	for k := range ps.Categories[cat] {
		if skipKey(cat, k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return categoryKeyOrder(cat, keys[i]) < categoryKeyOrder(cat, keys[j])
	})

	out := make([]htmlMetric, 0, len(keys))
	for _, k := range keys {
		m := ps.Categories[cat][k]
		val := formatMetricValue(m)
		if val == "-" {
			continue
		}
		out = append(out, htmlMetric{
			Label: metricLabel(cat, k),
			Value: val,
			Class: metricClass(cat, k, m),
		})
	}
	return out
}

func skipKey(cat Category, k Key) bool {
	s := string(k)
	if strings.HasSuffix(s, "_ticks") {
		return true
	}
	// The gauge + badge already represent these — skip in the breakdown table.
	if cat == Category("anti_cheat") && (k == Key("cheat_likelihood") || k == Key("cheater")) {
		return true
	}
	return false
}

// categoryKeyOrder gives an explicit display order for keys within important
// categories. Falls back to alphabetical for anything not listed.
func categoryKeyOrder(cat Category, k Key) string {
	preset := map[Category][]Key{
		Category("anti_cheat"): {
			Key("total_cheat_score"),
			Key("hs_score"),
			Key("snap_score"),
			Key("reaction_score"),
			Key("recoil_score"),
			Key("wingman_boost"),
			Key("competitive_boost"),
		},
		Category("kills"): {
			Key("total_kills"),
			Key("headshot_kills"),
			Key("headshot_percentage"),
		},
		Category("aiming"): {
			Key("snap_count"),
			Key("avg_snap_velocity"),
			Key("median_snap_velocity"),
			Key("p95_snap_velocity"),
		},
		Category("reaction"): {
			Key("reaction_samples"),
			Key("p10_reaction_time"),
			Key("median_reaction_time"),
			Key("sub_100ms_ratio"),
		},
		Category("game_info"): {
			Key("game_mode"),
			Key("round_count"),
		},
	}
	if list, ok := preset[cat]; ok {
		for i, key := range list {
			if key == k {
				return fmt.Sprintf("%03d", i)
			}
		}
	}
	return "z_" + string(k)
}

func metricLabel(_ Category, k Key) string {
	overrides := map[Key]string{
		Key("hs_score"):             "Headshot score",
		Key("snap_score"):           "Snap score",
		Key("reaction_score"):       "Reaction score",
		Key("recoil_score"):         "Recoil score",
		Key("total_cheat_score"):    "Combined score",
		Key("wingman_boost"):        "Wingman boost",
		Key("competitive_boost"):    "Competitive boost",
		Key("p95_snap_velocity"):    "P95 snap velocity",
		Key("avg_snap_velocity"):    "Avg snap velocity",
		Key("median_snap_velocity"): "Median snap velocity",
		Key("snap_count"):           "Snap count",
		Key("p10_reaction_time"):    "P10 reaction time",
		Key("median_reaction_time"): "Median reaction time",
		Key("sub_100ms_ratio"):      "Sub-100 ms ratio",
		Key("reaction_samples"):     "Reaction samples",
		Key("total_kills"):          "Total kills",
		Key("headshot_kills"):       "Headshot kills",
		Key("headshot_percentage"):  "Headshot %",
		Key("game_mode"):            "Game mode",
		Key("round_count"):          "Rounds",
		Key("knife_percentage"):     "Knife time",
		Key("non_knife_percentage"): "Weapon time",
		Key("no_weapon_percentage"): "Unarmed time",
	}
	if v, ok := overrides[k]; ok {
		return v
	}
	return titleize(strings.TrimSuffix(string(k), "_percentage"))
}

func metricClass(cat Category, k Key, m Metric) string {
	if m.Type == MetricString {
		if m.StringValue == "Yes" {
			return "hot"
		}
		if m.StringValue == "No" {
			return "dim"
		}
		return ""
	}

	if cat == Category("anti_cheat") && strings.HasSuffix(string(k), "_score") {
		if m.FloatValue >= 0.7 {
			return "hot"
		}
		if m.FloatValue >= 0.4 {
			return "warm"
		}
		return ""
	}

	if k == Key("headshot_percentage") {
		if m.FloatValue >= 70 {
			return "hot"
		}
		if m.FloatValue >= 55 {
			return "warm"
		}
	}
	if k == Key("p95_snap_velocity") {
		if m.FloatValue >= 3.0 {
			return "hot"
		}
		if m.FloatValue >= 2.0 {
			return "warm"
		}
	}
	if k == Key("p10_reaction_time") {
		if m.FloatValue > 0 && m.FloatValue <= 80 {
			return "hot"
		}
		if m.FloatValue > 0 && m.FloatValue <= 120 {
			return "warm"
		}
	}
	if k == Key("sub_100ms_ratio") {
		if m.FloatValue >= 20 {
			return "hot"
		}
		if m.FloatValue >= 10 {
			return "warm"
		}
	}
	return ""
}

func likelihoodClass(v float64) string {
	if v >= flagThreshold {
		return "flag"
	}
	if v >= amberThreshold {
		return "amber"
	}
	return "clean"
}

func titleize(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func fallback(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
