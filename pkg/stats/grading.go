package stats

// gradeBand is one threshold rung in a grading ladder. Bands are ordered from
// best to worst grade; the value compared against `edge` depends on whether
// the metric is higher-better or lower-better.
type gradeBand struct {
	edge  float64
	grade string
}

var (
	// Higher-is-better: edge is the lower bound for that grade.
	combatBands = []gradeBand{
		{1.40, "A+"},
		{1.20, "A"},
		{1.05, "B+"},
		{0.90, "B"},
		{0.75, "C+"},
		{0.55, "C"},
		{0.40, "D"},
	}
	grenadeBands = []gradeBand{
		{18, "A+"},
		{12, "A"},
		{8, "B+"},
		{5, "B"},
		{3, "C+"},
		{1, "C"},
		{0.5, "D"},
	}
	// Lower-is-better. TTD (sight → first damage) bands. Leetify's published
	// ranges put fast-pro at 400–500 ms and most clean players at 500–600 ms,
	// so the ladder is shifted accordingly. Anything <350 ms is exceptional;
	// <200 ms across many engagements is statistically implausible (cheat
	// signal, not skill).
	reactionBands = []gradeBand{
		{350, "A+"},
		{450, "A"},
		{550, "B+"},
		{650, "B"},
		{750, "C+"},
		{850, "C"},
		{950, "D"},
	}
	recoilBands = []gradeBand{
		{0.6, "A+"},
		{0.8, "A"},
		{1.0, "B+"},
		{1.2, "B"},
		{1.4, "C+"},
		{1.7, "C"},
		{2.0, "D"},
	}
)

func gradeHigher(v float64, bands []gradeBand) string {
	for _, b := range bands {
		if v >= b.edge {
			return b.grade
		}
	}
	return "F"
}

func gradeLower(v float64, bands []gradeBand) string {
	for _, b := range bands {
		if v <= b.edge {
			return b.grade
		}
	}
	return "F"
}

// gradeRank assigns a numeric rank to each grade so we can average them.
var gradeRank = map[string]int{
	"A+": 7, "A": 6, "B+": 5, "B": 4, "C+": 3, "C": 2, "D": 1, "F": 0,
}
var rankToGrade = []string{"F", "D", "C", "C+", "B", "B+", "A", "A+"}

func averageGrade(grades []string) string {
	if len(grades) == 0 {
		return ""
	}
	sum, n := 0, 0
	for _, g := range grades {
		if r, ok := gradeRank[g]; ok {
			sum += r
			n++
		}
	}
	if n == 0 {
		return ""
	}
	// Round to nearest rank: integer floor with +0.5 bias via integer math.
	avg := (2*sum + n) / (2 * n)
	if avg < 0 {
		avg = 0
	}
	if avg >= len(rankToGrade) {
		avg = len(rankToGrade) - 1
	}
	return rankToGrade[avg]
}

// GradingCollector emits per-player A+..F grades across the skill-bearing
// metric categories plus a composite Overall grade. Runs last so it can read
// the final values produced by every upstream collector.
type GradingCollector struct {
	*BaseCollector
}

func NewGradingCollector() *GradingCollector {
	return &GradingCollector{
		BaseCollector: NewBaseCollector("Grading", Category("rating")),
	}
}

func (g *GradingCollector) CollectFinalStats(demoStats *DemoStats) {
	for sid, ps := range demoStats.Players {
		if sid == 0 {
			continue
		}
		grades := make([]string, 0, 4)

		// Combat — K/D from scoreboard. Need at least one death to be honest;
		// otherwise we'd hand out A+ for a one-engagement sample.
		kills := intMetric(ps, scoreboardCategory, Key("kills"))
		deaths := intMetric(ps, scoreboardCategory, Key("deaths"))
		if deaths > 0 {
			grade := gradeHigher(float64(kills)/float64(deaths), combatBands)
			ps.AddMetric(Category("kills"), Key("grade"), Metric{
				Type: MetricString, StringValue: grade,
				Description: "Combat grade — K/D ratio",
			})
			grades = append(grades, grade)
		}

		// Reaction — P10 sight-to-shot in ms.
		if m, ok := ps.GetMetric(Category("reaction"), Key("p10_ttd")); ok && m.FloatValue > 0 {
			grade := gradeLower(m.FloatValue, reactionBands)
			ps.AddMetric(Category("reaction"), Key("grade"), Metric{
				Type: MetricString, StringValue: grade,
				Description: "Reaction grade — P10 sight-to-shot",
			})
			grades = append(grades, grade)
		}

		// Recoil — mean angular error in degrees.
		if m, ok := ps.GetMetric(Category("recoil"), Key("mean_angular_error")); ok && m.FloatValue > 0 {
			grade := gradeLower(m.FloatValue, recoilBands)
			ps.AddMetric(Category("recoil"), Key("grade"), Metric{
				Type: MetricString, StringValue: grade,
				Description: "Recoil grade — mean angular error",
			})
			grades = append(grades, grade)
		}

		// Grenades — HE damage per round.
		if m, ok := ps.GetMetric(grenadeCategory, Key("damage_per_round")); ok && m.FloatValue > 0 {
			grade := gradeHigher(m.FloatValue, grenadeBands)
			ps.AddMetric(grenadeCategory, Key("grade"), Metric{
				Type: MetricString, StringValue: grade,
				Description: "Grenade grade — HE damage per round",
			})
			grades = append(grades, grade)
		}

		if overall := averageGrade(grades); overall != "" {
			ps.AddMetric(Category("rating"), Key("overall"), Metric{
				Type: MetricString, StringValue: overall,
				Description: "Overall skill grade — average across category grades",
			})
		}
	}
}
