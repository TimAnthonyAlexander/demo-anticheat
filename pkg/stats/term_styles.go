package stats

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Palette mirrors the HTML CSS custom properties in report.tmpl.html so the
// terminal output is visually consistent with the HTML report.
var (
	colorFlag    = lipgloss.Color("#dc5a4a")
	colorWarn    = lipgloss.Color("#d49a3a")
	colorOk      = lipgloss.Color("#4f9d65")
	colorOkLight = lipgloss.Color("#7ec894")
	colorOrange  = lipgloss.Color("#d97a4a")
	colorText    = lipgloss.Color("#e6e7e9")
	colorDim     = lipgloss.Color("#9aa0a8")
	colorFaint   = lipgloss.Color("#6a707a")
	colorLine    = lipgloss.Color("#3a4048")
	colorCT      = lipgloss.Color("#6fa8c8")
)

type styles struct {
	r     *lipgloss.Renderer
	isTTY bool

	header        lipgloss.Style
	headerName    lipgloss.Style
	demoTitle     lipgloss.Style
	meta          lipgloss.Style
	metaCode      lipgloss.Style
	verdict       lipgloss.Style
	verdictFlag   lipgloss.Style
	verdictClean  lipgloss.Style
	verdictDetail lipgloss.Style

	sectionTitle lipgloss.Style

	teamLabelT  lipgloss.Style
	teamLabelCT lipgloss.Style
	teamCount   lipgloss.Style
	tableHeader lipgloss.Style
	tableNum    lipgloss.Style
	tableMuted  lipgloss.Style
	tableName   lipgloss.Style

	cardOK     lipgloss.Style
	cardFlag   lipgloss.Style
	cardBorder lipgloss.Border
	plyrName  lipgloss.Style
	plyrID    lipgloss.Style
	likeFlag  lipgloss.Style
	likeWarn  lipgloss.Style
	likeOk    lipgloss.Style
	flagBadge lipgloss.Style
	okBadge   lipgloss.Style

	gradeA lipgloss.Style
	gradeB lipgloss.Style
	gradeC lipgloss.Style
	gradeD lipgloss.Style
	gradeF lipgloss.Style

	gradePillTitle   lipgloss.Style
	overallPillTitle lipgloss.Style

	subhead lipgloss.Style

	chLabel      lipgloss.Style
	chLabelMuted lipgloss.Style
	chScore      lipgloss.Style
	chConf       lipgloss.Style
	zoneHot      lipgloss.Style
	zoneWarm     lipgloss.Style
	zoneOk       lipgloss.Style
	zoneNone     lipgloss.Style

	barHot   lipgloss.Style
	barWarm  lipgloss.Style
	barOk    lipgloss.Style
	barFaint lipgloss.Style
	barTrack lipgloss.Style

	boostLabel lipgloss.Style
	boostValue lipgloss.Style
	boostHot   lipgloss.Style
	boostWarm  lipgloss.Style

	categoryTitle lipgloss.Style
	categoryNote  lipgloss.Style
	metricLabel   lipgloss.Style
	metricValue   lipgloss.Style
	metricHot     lipgloss.Style
	metricWarm    lipgloss.Style

	footer lipgloss.Style
}

// newStyles constructs the style table bound to a fresh lipgloss renderer
// targeting w. When isTTY is false the renderer is locked to the Ascii
// profile, so all color/styling is silently stripped on the wire.
func newStyles(w io.Writer, isTTY bool) *styles {
	r := lipgloss.NewRenderer(w)
	if !isTTY {
		r.SetColorProfile(termenv.Ascii)
	}

	s := &styles{r: r, isTTY: isTTY}
	ns := r.NewStyle

	s.header = ns().Foreground(colorFaint)
	s.headerName = ns().Foreground(colorDim)
	s.demoTitle = ns().Foreground(colorText).Bold(true)
	s.meta = ns().Foreground(colorDim)
	s.metaCode = ns().Foreground(colorText)
	s.verdict = ns().Foreground(colorText)
	s.verdictFlag = ns().Foreground(colorFlag).Bold(true)
	s.verdictClean = ns().Foreground(colorOk).Bold(true)
	s.verdictDetail = ns().Foreground(colorDim)

	s.sectionTitle = ns().Foreground(colorFaint).Bold(true)

	s.teamLabelT = ns().Foreground(colorWarn).Bold(true)
	s.teamLabelCT = ns().Foreground(colorCT).Bold(true)
	s.teamCount = ns().Foreground(colorFaint)
	s.tableHeader = ns().Foreground(colorFaint).Bold(true)
	s.tableNum = ns().Foreground(colorText)
	s.tableMuted = ns().Foreground(colorFaint)
	s.tableName = ns().Foreground(colorText)

	border := lipgloss.RoundedBorder()
	if !isTTY {
		border = lipgloss.NormalBorder()
	}
	cardBase := ns().
		Border(border).
		Padding(1, 2)
	s.cardOK = cardBase.BorderForeground(colorLine)
	s.cardFlag = cardBase.BorderForeground(colorFlag)
	s.cardBorder = border

	s.plyrName = ns().Foreground(colorText).Bold(true)
	s.plyrID = ns().Foreground(colorFaint)
	s.likeFlag = ns().Foreground(colorFlag).Bold(true)
	s.likeWarn = ns().Foreground(colorWarn).Bold(true)
	s.likeOk = ns().Foreground(colorOk).Bold(true)
	s.flagBadge = ns().
		Foreground(colorFlag).
		Bold(true).
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, false, false)
	s.okBadge = ns().Foreground(colorFaint)

	s.gradeA = ns().Foreground(colorOk).Bold(true)
	s.gradeB = ns().Foreground(colorOkLight).Bold(true)
	s.gradeC = ns().Foreground(colorWarn).Bold(true)
	s.gradeD = ns().Foreground(colorOrange).Bold(true)
	s.gradeF = ns().Foreground(colorFlag).Bold(true)

	s.gradePillTitle = ns().Foreground(colorFaint)
	s.overallPillTitle = ns().Foreground(colorText).Bold(true)

	s.subhead = ns().Foreground(colorFaint).Bold(true)

	s.chLabel = ns().Foreground(colorDim)
	s.chLabelMuted = ns().Foreground(colorFaint)
	s.chScore = ns().Foreground(colorText)
	s.chConf = ns().Foreground(colorFaint)
	s.zoneHot = ns().Foreground(colorFlag).Bold(true)
	s.zoneWarm = ns().Foreground(colorWarn).Bold(true)
	s.zoneOk = ns().Foreground(colorOk)
	s.zoneNone = ns().Foreground(colorFaint)

	s.barHot = ns().Foreground(colorFlag)
	s.barWarm = ns().Foreground(colorWarn)
	s.barOk = ns().Foreground(colorOk)
	s.barFaint = ns().Foreground(colorFaint)
	s.barTrack = ns().Foreground(colorLine)

	s.boostLabel = ns().Foreground(colorFaint)
	s.boostValue = ns().Foreground(colorText)
	s.boostHot = ns().Foreground(colorFlag).Bold(true)
	s.boostWarm = ns().Foreground(colorWarn)

	s.categoryTitle = ns().Foreground(colorFaint).Bold(true)
	s.categoryNote = ns().Foreground(colorWarn)
	s.metricLabel = ns().Foreground(colorDim)
	s.metricValue = ns().Foreground(colorText)
	s.metricHot = ns().Foreground(colorFlag)
	s.metricWarm = ns().Foreground(colorWarn)

	s.footer = ns().Foreground(colorFaint)

	return s
}

// gradeStyle returns the badge style for a grade class string. The class
// names ("grade-a", "grade-b", ...) come from gradeClass() in
// html_reporter.go and are reused verbatim so both reporters stay in sync.
func (s *styles) gradeStyle(class string) lipgloss.Style {
	switch class {
	case "grade-a":
		return s.gradeA
	case "grade-b":
		return s.gradeB
	case "grade-c":
		return s.gradeC
	case "grade-d":
		return s.gradeD
	default:
		return s.gradeF
	}
}

// zoneStyle returns the badge style for a zone class produced by
// zoneClass() in html_reporter.go.
func (s *styles) zoneStyle(class string) lipgloss.Style {
	switch class {
	case "zone-hot":
		return s.zoneHot
	case "zone-warm":
		return s.zoneWarm
	case "zone-ok":
		return s.zoneOk
	default:
		return s.zoneNone
	}
}

// barStyle returns the fill style for a zone class.
func (s *styles) barStyle(class string) lipgloss.Style {
	switch class {
	case "zone-hot":
		return s.barHot
	case "zone-warm":
		return s.barWarm
	case "zone-ok":
		return s.barOk
	default:
		return s.barFaint
	}
}

// likelihoodStyle returns the style for the big likelihood number based on
// the class produced by likelihoodClass() in html_reporter.go.
func (s *styles) likelihoodStyle(class string) lipgloss.Style {
	switch class {
	case "flag":
		return s.likeFlag
	case "warn":
		return s.likeWarn
	default:
		return s.likeOk
	}
}

// metricStyle returns the style for a category metric value based on the
// class produced by metricClass() in html_reporter.go.
func (s *styles) metricStyle(class string) lipgloss.Style {
	switch class {
	case "hot":
		return s.metricHot
	case "warm":
		return s.metricWarm
	default:
		return s.metricValue
	}
}

// boostStyle returns the style for a boost value based on the class
// produced by metricClass().
func (s *styles) boostStyle(class string) lipgloss.Style {
	switch class {
	case "hot":
		return s.boostHot
	case "warm":
		return s.boostWarm
	default:
		return s.boostValue
	}
}
