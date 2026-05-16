package stats

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
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
	warnThreshold    = 25.0
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
	LowestLikelihood  float64
	LowestName        string
	GameMode          string
	RoundCount        int64
	MetricCount       int
	Teams             []htmlTeam
	Players           []htmlPlayer
}

type htmlTeam struct {
	Label   string
	Players []htmlScoreRow
}

type htmlScoreRow struct {
	Name    string
	Kills   string
	Deaths  string
	Assists string
	ADR     string
	HS      string
	MVPs    string
	// sortKills is unexported but used for ordering before render.
	sortKills int64
}

type htmlPlayer struct {
	Name            string
	SteamID         string
	Likelihood      float64
	LikelihoodClass string
	Flagged         bool
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

	for i, ps := range realPlayers {
		hp := buildPlayer(ps)
		if hp.Flagged {
			data.FlaggedCount++
		}
		if i == 0 || hp.Likelihood > data.HighestLikelihood {
			data.HighestLikelihood = hp.Likelihood
			data.HighestName = ps.Player.Name
		}
		if i == 0 || hp.Likelihood < data.LowestLikelihood {
			data.LowestLikelihood = hp.Likelihood
			data.LowestName = ps.Player.Name
		}
		for _, cat := range hp.Categories {
			metricCount += len(cat.Metrics)
		}
		data.Players = append(data.Players, hp)
	}

	data.MetricCount = metricCount
	data.Teams = buildScoreboard(realPlayers)
	return data
}

func buildScoreboard(players []*PlayerStats) []htmlTeam {
	groups := map[string][]htmlScoreRow{}
	order := []string{"T", "CT"}

	for _, ps := range players {
		row, side := buildScoreRow(ps)
		if row.sortKills == 0 && row.Deaths == "0" && row.ADR == "—" && row.Assists == "0" {
			// No scoreboard activity at all — skip (probably a spectator or
			// placeholder slot).
			continue
		}
		groups[side] = append(groups[side], row)
	}

	out := make([]htmlTeam, 0, 2)
	for _, side := range order {
		rows := groups[side]
		if len(rows) == 0 {
			continue
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].sortKills > rows[j].sortKills })
		out = append(out, htmlTeam{Label: side, Players: rows})
	}

	// Fall back to a single "All" table if no team side was recorded.
	if len(out) == 0 {
		if rows := groups[""]; len(rows) > 0 {
			sort.Slice(rows, func(i, j int) bool { return rows[i].sortKills > rows[j].sortKills })
			out = append(out, htmlTeam{Label: "All", Players: rows})
		}
	}
	return out
}

func buildScoreRow(ps *PlayerStats) (htmlScoreRow, string) {
	side := ""
	if m, ok := ps.GetMetric(scoreboardCategory, Key("team")); ok {
		side = m.StringValue
	}

	kills := intMetric(ps, scoreboardCategory, Key("kills"))
	deaths := intMetric(ps, scoreboardCategory, Key("deaths"))
	assists := intMetric(ps, scoreboardCategory, Key("assists"))
	mvps := intMetric(ps, scoreboardCategory, Key("mvps"))

	adr := "—"
	if m, ok := ps.GetMetric(scoreboardCategory, Key("adr")); ok && m.FloatValue > 0 {
		adr = fmt.Sprintf("%.1f", m.FloatValue)
	}
	hs := "—"
	if m, ok := ps.GetMetric(scoreboardCategory, Key("hs_percentage")); ok && kills > 0 {
		hs = fmt.Sprintf("%.0f%%", m.FloatValue)
	}

	row := htmlScoreRow{
		Name:      fallback(ps.Player.Name, "Unknown"),
		Kills:     fmt.Sprintf("%d", kills),
		Deaths:    fmt.Sprintf("%d", deaths),
		Assists:   fmt.Sprintf("%d", assists),
		ADR:       adr,
		HS:        hs,
		MVPs:      fmt.Sprintf("%d", mvps),
		sortKills: kills,
	}
	return row, side
}

func intMetric(ps *PlayerStats, cat Category, k Key) int64 {
	if m, ok := ps.GetMetric(cat, k); ok {
		return m.IntValue
	}
	return 0
}

func buildPlayer(ps *PlayerStats) htmlPlayer {
	likelihood := getMetricFloatValue(ps, Category("anti_cheat"), Key("cheat_likelihood"))
	flagged := false
	if m, found := ps.GetMetric(Category("anti_cheat"), Key("cheater")); found && m.StringValue == "Yes" {
		flagged = true
	}

	return htmlPlayer{
		Name:            fallback(ps.Player.Name, "Unknown"),
		SteamID:         fmt.Sprintf("%d", ps.Player.SteamID64),
		Likelihood:      likelihood,
		LikelihoodClass: likelihoodClass(likelihood),
		Flagged:         flagged,
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
	{Category("rating"), "Skill Rating", ""},
	{Category("anti_cheat"), "Score Breakdown", ""},
	{Category("kills"), "Combat", ""},
	{Category("aiming"), "Aim Snap", ""},
	{Category("reaction"), "Reaction Time", ""},
	{Category("recoil"), "Recoil Control", ""},
	{Category("weapons"), "Weapon Usage", ""},
	{Category("utility"), "Grenades", ""},
	{Category("behavioral"), "Behavioral", "informational"},
	{Category("game_info"), "Game Info", ""},
}

func buildCategories(ps *PlayerStats) []htmlCategory {
	out := make([]htmlCategory, 0, len(categoryDisplay))
	seen := make(map[Category]bool)
	// scoreboard is rendered in its own section above the cards.
	seen[scoreboardCategory] = true

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
			Key("position_discount"),
		},
		Category("kills"): {
			Key("grade"),
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
		Category("recoil"): {
			Key("grade"),
			Key("mean_angular_error"),
			Key("burst_count"),
			Key("total_counted_bullets"),
			Key("total_error_sum"),
			Key("recoil_interpretation"),
		},
		Category("rating"): {
			Key("overall"),
		},
		Category("reaction"): {
			Key("grade"),
			Key("reaction_samples"),
			Key("p10_reaction_time"),
			Key("median_reaction_time"),
			Key("sub_100ms_ratio"),
		},
		Category("game_info"): {
			Key("game_mode"),
			Key("round_count"),
		},
		Category("utility"): {
			Key("grade"),
			Key("thrown"),
			Key("damage"),
			Key("damage_per_throw"),
			Key("enemies_per_throw"),
			Key("damage_per_round"),
			Key("killed"),
			Key("he_detonated"),
			Key("he_zero_damage"),
			Key("enemy_hits"),
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

// weaponLabels maps each known recoil-tracked weapon's lowercase key prefix
// to its canonical CS-side display name.
var weaponLabels = map[string]string{
	"ak47": "AK-47",
	"m4a4": "M4A4",
	"m4a1": "M4A1-S",
	"mp9":  "MP9",
	"p90":  "P90",
}

func metricLabel(_ Category, k Key) string {
	s := string(k)
	// Per-weapon recoil keys are emitted as "{weapon}_{suffix}" — handle them
	// in one place so we don't enumerate every weapon × suffix combo.
	for prefix, display := range weaponLabels {
		if !strings.HasPrefix(s, prefix+"_") {
			continue
		}
		suffix := s[len(prefix)+1:]
		switch suffix {
		case "shots":
			return display + " shots"
		case "bullets":
			return display + " burst bullets"
		case "efficiency":
			return display + " efficiency"
		case "mean_error":
			return display + " mean error"
		case "error_sum":
			return display + " error sum"
		}
	}

	overrides := map[Key]string{
		Key("hs_score"):             "Headshot score",
		Key("snap_score"):           "Snap score",
		Key("reaction_score"):       "Reaction score",
		Key("recoil_score"):         "Recoil score",
		Key("total_cheat_score"):    "Combined score",
		Key("wingman_boost"):        "Wingman boost",
		Key("competitive_boost"):    "Competitive boost",
		Key("position_discount"):    "Position discount",
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
		Key("thrown"):               "Thrown",
		Key("damage"):               "Damage",
		Key("enemy_hits"):           "Enemy hits",
		Key("damage_per_throw"):     "Damage per throw",
		Key("enemies_per_throw"):    "Enemies damaged per throw",
		Key("damage_per_round"):     "Damage per round",
		Key("killed"):               "Killed",
		Key("he_detonated"):         "HE detonated",
		Key("he_zero_damage"):       "HE with 0 damage",
		Key("grade"):                "Grade",
		Key("overall"):              "Overall grade",
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

	switch k {
	case Key("headshot_percentage"):
		if m.FloatValue >= 70 {
			return "hot"
		}
		if m.FloatValue >= 55 {
			return "warm"
		}
	case Key("p95_snap_velocity"):
		if m.FloatValue >= 3.0 {
			return "hot"
		}
		if m.FloatValue >= 2.0 {
			return "warm"
		}
	case Key("p10_reaction_time"):
		if m.FloatValue > 0 && m.FloatValue <= 80 {
			return "hot"
		}
		if m.FloatValue > 0 && m.FloatValue <= 120 {
			return "warm"
		}
	case Key("sub_100ms_ratio"):
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
	if v >= warnThreshold {
		return "warn"
	}
	return "ok"
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
