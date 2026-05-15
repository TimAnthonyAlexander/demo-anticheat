package analyzer

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/timanthonyalexander/demo-anticheat/pkg/stats"
)

// Ground-truth labels gathered from manual demo review (see MEMORY.md).
//
// Wingman demo: walls_wingman.dem
//   - SteamID 76561199383848692 ("wingman ANDY")     — wallhacker
//   - SteamID 76561199558035269 ("キルア")              — wallhacker
//   - SteamID 76561198175724267 ("⚚_NaYp1x_⚚")       — clean
//   - SteamID 76561199801779382 ("𝖋𝖊𝖓𝖊𝖘𝖍𝖊")          — clean
//
// SHADE vs Kultywator demo: 5v5 league/tournament, all 10 players confirmed clean.
var (
	wingmanCheaters = map[uint64]string{
		76561199383848692: "wingman ANDY",
		76561199558035269: "キルア",
	}
	wingmanClean = map[uint64]string{
		76561198175724267: "⚚_NaYp1x_⚚",
		76561199801779382: "𝖋𝖊𝖓𝖊𝖘𝖍𝖊",
	}
)

const (
	wingmanDemoPath = "../../demos/walls_wingman.dem"
	ancientDemoPath = "/Users/tim.alexander/Downloads/2026-05-10_13-38-29_1_de_ancient_SHADE_vs_Kultywator_Stara_Krobia.dem"

	// Required separation between the lowest-scoring known cheater and the
	// highest-scoring known clean player. Keeps tuning honest — we want a
	// gap, not a hairline.
	requiredMargin = 10.0
)

type playerScore struct {
	steamID    uint64
	name       string
	likelihood float64
}

func runAnalyze(t *testing.T, demoPath string) map[uint64]playerScore {
	t.Helper()

	abs, err := filepath.Abs(demoPath)
	if err != nil {
		t.Fatalf("resolve %s: %v", demoPath, err)
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		t.Skipf("demo %s not present, skipping", abs)
	}

	results, err := NewAnalyzer(abs).Analyze()
	if err != nil {
		t.Fatalf("analyze %s: %v", abs, err)
	}

	scores := make(map[uint64]playerScore, len(results.DemoStats.Players))
	for sid, ps := range results.DemoStats.Players {
		if sid == 0 {
			continue // "Unknown" placeholder row
		}
		m, ok := ps.GetMetric(stats.Category("anti_cheat"), stats.Key("cheat_likelihood"))
		if !ok {
			t.Fatalf("player %d (%s) has no cheat_likelihood metric", sid, ps.Player.Name)
		}
		scores[sid] = playerScore{steamID: sid, name: ps.Player.Name, likelihood: m.FloatValue}
	}
	return scores
}

func dumpRanked(t *testing.T, label string, scores map[uint64]playerScore, cheaters map[uint64]string) {
	t.Helper()
	list := make([]playerScore, 0, len(scores))
	for _, s := range scores {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].likelihood > list[j].likelihood })

	t.Logf("=== %s ranked by cheat likelihood ===", label)
	for _, s := range list {
		tag := ""
		if _, isCheat := cheaters[s.steamID]; isCheat {
			tag = " [CHEATER]"
		}
		t.Logf("  %6.2f%%  %s%s", s.likelihood, s.name, tag)
	}
}

// TestDetector_DumpBehavioral is diagnostic-only; it prints raw behavioral
// metrics for both demos so we can calibrate the combiner empirically.
func TestDetector_DumpBehavioral(t *testing.T) {
	for _, tc := range []struct {
		label    string
		path     string
		cheaters map[uint64]string
	}{
		{"wingman", wingmanDemoPath, wingmanCheaters},
		{"pros", ancientDemoPath, nil},
	} {
		abs, err := filepath.Abs(tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			t.Logf("skipping %s (demo missing)", tc.label)
			continue
		}
		results, err := NewAnalyzer(abs).Analyze()
		if err != nil {
			t.Fatalf("%s analyze: %v", tc.label, err)
		}

		type row struct {
			name              string
			isCheat           bool
			backPct           float64
			backDeaths        int64
			preFOV            float64
			preFOVN           int64
			attention         float64
			attentionN        int64
		}
		rows := []row{}
		for sid, ps := range results.DemoStats.Players {
			if sid == 0 {
				continue
			}
			r := row{name: ps.Player.Name}
			if _, ok := tc.cheaters[sid]; ok {
				r.isCheat = true
			}
			if m, ok := ps.GetMetric(stats.Category("behavioral"), stats.Key("back_killed_pct")); ok {
				r.backPct = m.FloatValue
			}
			if m, ok := ps.GetMetric(stats.Category("behavioral"), stats.Key("back_killed_total_deaths")); ok {
				r.backDeaths = m.IntValue
			}
			if m, ok := ps.GetMetric(stats.Category("behavioral"), stats.Key("pre_fov_aim_median_deg")); ok {
				r.preFOV = m.FloatValue
			}
			if m, ok := ps.GetMetric(stats.Category("behavioral"), stats.Key("pre_fov_aim_samples")); ok {
				r.preFOVN = m.IntValue
			}
			if m, ok := ps.GetMetric(stats.Category("behavioral"), stats.Key("nearest_enemy_angle_median_deg")); ok {
				r.attention = m.FloatValue
			}
			if m, ok := ps.GetMetric(stats.Category("behavioral"), stats.Key("nearest_enemy_angle_samples")); ok {
				r.attentionN = m.IntValue
			}
			rows = append(rows, r)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

		t.Logf("--- %s behavioral metrics ---", tc.label)
		t.Logf("  %-22s %-9s %-9s %-9s %-9s %-9s %-9s %s",
			"name", "back%", "deaths", "preFOV°", "preN", "attn°", "attnN", "tag")
		for _, r := range rows {
			tag := ""
			if r.isCheat {
				tag = "[CHEATER]"
			}
			t.Logf("  %-22s %-9.2f %-9d %-9.2f %-9d %-9.2f %-9d %s",
				r.name, r.backPct, r.backDeaths, r.preFOV, r.preFOVN, r.attention, r.attentionN, tag)
		}
	}
}

// TestDetector_WingmanCheatersAboveClean ensures the two known wingman wallhackers
// score strictly above both clean wingman teammates.
func TestDetector_WingmanCheatersAboveClean(t *testing.T) {
	scores := runAnalyze(t, wingmanDemoPath)
	dumpRanked(t, "wingman", scores, wingmanCheaters)

	minCheater, maxCheaterName, foundCheater := minScoreIn(scores, wingmanCheaters)
	maxClean, maxCleanName, foundClean := maxScoreIn(scores, wingmanClean)
	if !foundCheater {
		t.Fatal("no wingman cheaters found in scored players")
	}
	if !foundClean {
		t.Fatal("no clean wingman players found in scored players")
	}

	if minCheater <= maxClean {
		t.Errorf("wingman cheater/clean ordering broken: lowest cheater %q=%.2f%% must exceed highest clean %q=%.2f%%",
			maxCheaterName, minCheater, maxCleanName, maxClean)
	}
}

// TestDetector_CheatersAboveAllPros ensures the wingman cheaters score higher than
// every clean pro in the SHADE vs Kultywator demo, with a configurable margin so
// tuning has to produce real separation rather than chasing decimals.
func TestDetector_CheatersAboveAllPros(t *testing.T) {
	wingman := runAnalyze(t, wingmanDemoPath)
	pros := runAnalyze(t, ancientDemoPath)
	dumpRanked(t, "wingman", wingman, wingmanCheaters)
	dumpRanked(t, "pros (ancient)", pros, nil)

	minCheater, minCheaterName, found := minScoreIn(wingman, wingmanCheaters)
	if !found {
		t.Fatal("no wingman cheaters found")
	}

	var maxPro float64
	var maxProName string
	for _, s := range pros {
		if s.likelihood > maxPro {
			maxPro = s.likelihood
			maxProName = s.name
		}
	}

	gap := minCheater - maxPro
	t.Logf("min cheater %q = %.2f%%, max pro %q = %.2f%%, gap = %.2f%% (required %.2f)",
		minCheaterName, minCheater, maxProName, maxPro, gap, requiredMargin)

	if gap < requiredMargin {
		t.Errorf("cheater/pro separation %.2f%% below required margin %.2f%%", gap, requiredMargin)
	}
}

// flagThreshold mirrors the value used in cheat_detection.go. Kept here as a
// constant rather than imported because the production constant lives in a
// different package's unexported scope.
const flagThreshold = 50.0

// TestDetector_CleanProsBelowFlagThreshold ensures none of the confirmed-clean pros
// would be auto-flagged as cheaters.
func TestDetector_CleanProsBelowFlagThreshold(t *testing.T) {
	pros := runAnalyze(t, ancientDemoPath)

	for _, s := range pros {
		if s.likelihood >= flagThreshold {
			t.Errorf("clean pro %q falsely flagged: %.2f%% >= %.2f%%", s.name, s.likelihood, flagThreshold)
		}
	}
}

// TestDetector_BothWingmanCheatersFlagged asserts BOTH known wingman cheaters
// are auto-flagged (Cheater=Yes). Catches regressions that drop either score
// back below the threshold.
func TestDetector_BothWingmanCheatersFlagged(t *testing.T) {
	wingman := runAnalyze(t, wingmanDemoPath)

	for sid, name := range wingmanCheaters {
		s, ok := wingman[sid]
		if !ok {
			t.Errorf("known cheater %q (sid %d) missing from analysis", name, sid)
			continue
		}
		if s.likelihood < flagThreshold {
			t.Errorf("known cheater %q only scored %.2f%%, below flag threshold %.0f%%",
				s.name, s.likelihood, flagThreshold)
		}
	}
}

// TestDetector_CleanWingmanBelowFlagThreshold ensures the two clean wingman
// teammates of the wallhackers are not falsely flagged.
func TestDetector_CleanWingmanBelowFlagThreshold(t *testing.T) {
	wingman := runAnalyze(t, wingmanDemoPath)

	for sid, name := range wingmanClean {
		s, ok := wingman[sid]
		if !ok {
			continue
		}
		if s.likelihood >= flagThreshold {
			t.Errorf("clean wingman player %q (%s) falsely flagged: %.2f%% >= %.2f%%",
				name, s.name, s.likelihood, flagThreshold)
		}
	}
}

func minScoreIn(scores map[uint64]playerScore, ids map[uint64]string) (float64, string, bool) {
	min := 0.0
	name := ""
	found := false
	for sid := range ids {
		s, ok := scores[sid]
		if !ok {
			continue
		}
		if !found || s.likelihood < min {
			min = s.likelihood
			name = s.name
			found = true
		}
	}
	if !found {
		return 0, "", false
	}
	return min, name, true
}

func maxScoreIn(scores map[uint64]playerScore, ids map[uint64]string) (float64, string, bool) {
	max := 0.0
	name := ""
	found := false
	for sid := range ids {
		s, ok := scores[sid]
		if !ok {
			continue
		}
		if !found || s.likelihood > max {
			max = s.likelihood
			name = s.name
			found = true
		}
	}
	if !found {
		return 0, "", false
	}
	return max, name, true
}

