package stats

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderPlayerCard renders one player as a bordered card. innerWidth is the
// usable width inside the card (after subtracting borders + padding).
func renderPlayerCard(s *styles, p htmlPlayer, innerWidth int) string {
	if innerWidth < 40 {
		innerWidth = 40
	}

	body := strings.Builder{}
	body.WriteString(renderCardHead(s, p, innerWidth))
	body.WriteString("\n\n")

	if len(p.Grades) > 0 || p.OverallGrade != "" {
		body.WriteString(renderGradesRow(s, p, innerWidth))
		body.WriteString("\n\n")
	}

	if len(p.Channels) > 0 {
		body.WriteString(renderChannelsBlock(s, p.Channels, innerWidth))
		body.WriteString("\n\n")
	}

	if len(p.Boosts) > 0 {
		body.WriteString(renderBoostsStrip(s, p.Boosts, innerWidth))
		body.WriteString("\n\n")
	}

	if len(p.Categories) > 0 {
		body.WriteString(renderCategoriesGrid(s, p.Categories, innerWidth))
	}

	card := s.cardOK
	if p.Flagged {
		card = s.cardFlag
	}
	return card.Width(innerWidth + 4).Render(strings.TrimRight(body.String(), "\n"))
}

func renderCardHead(s *styles, p htmlPlayer, innerWidth int) string {
	name := s.plyrName.Render(p.Name)
	id := s.plyrID.Render("steam " + p.SteamID)
	left := lipgloss.JoinVertical(lipgloss.Left, name, id)

	pct := fmt.Sprintf("%.1f%%", p.Likelihood)
	likeStr := s.likelihoodStyle(p.LikelihoodClass).Render(pct)
	badge := s.okBadge.Render("clear")
	if p.Flagged {
		badge = s.flagBadge.Render("FLAGGED")
	}
	right := lipgloss.JoinVertical(lipgloss.Right, likeStr, badge)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := innerWidth - leftW - rightW
	if gap < 1 {
		gap = 1
	}
	spacer := s.r.NewStyle().Width(gap).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, right)
}

func renderGradesRow(s *styles, p htmlPlayer, innerWidth int) string {
	pills := make([]string, 0, len(p.Grades)+1)
	for _, g := range p.Grades {
		pills = append(pills, gradePill(s, g.Title, g.Grade, s.gradeStyle(g.Class), false))
	}
	if p.OverallGrade != "" {
		pills = append(pills, gradePill(s, "Overall", p.OverallGrade, s.gradeStyle(p.OverallGradeClass), true))
	}

	if len(pills) == 0 {
		return ""
	}
	return joinPillsWrap(s, pills, innerWidth)
}

func gradePill(s *styles, title, grade string, gradeStyle lipgloss.Style, overall bool) string {
	titleStyle := s.gradePillTitle
	if overall {
		titleStyle = s.overallPillTitle
	}
	t := titleStyle.Render(strings.ToUpper(title))
	g := gradeStyle.Render(grade)
	stack := lipgloss.JoinVertical(lipgloss.Center, t, g)
	pill := s.r.NewStyle().
		Border(s.cardBorder).
		BorderForeground(colorLine).
		Padding(0, 1)
	return pill.Render(stack)
}

// joinPillsWrap joins pills horizontally with a 1-cell gap, wrapping to a
// new row when the running width would exceed innerWidth.
func joinPillsWrap(s *styles, pills []string, innerWidth int) string {
	rows := make([][]string, 0)
	current := make([]string, 0)
	cw := 0
	for _, p := range pills {
		w := lipgloss.Width(p)
		needed := w
		if len(current) > 0 {
			needed += 1
		}
		if cw+needed > innerWidth && len(current) > 0 {
			rows = append(rows, current)
			current = []string{p}
			cw = w
			continue
		}
		current = append(current, p)
		cw += needed
	}
	if len(current) > 0 {
		rows = append(rows, current)
	}
	joined := make([]string, 0, len(rows))
	for _, row := range rows {
		joined = append(joined, lipgloss.JoinHorizontal(lipgloss.Top, interleaveGap(row, " ")...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, joined...)
}

const (
	chLabelW = 22
	chBarW   = 14
	chPctW   = 4
	chConfW  = 9
	chZoneW  = 8
)

func renderChannelsBlock(s *styles, channels []htmlChannel, _ int) string {
	var b strings.Builder
	b.WriteString(s.subhead.Render("CHEAT-DETECTION CHANNELS"))
	b.WriteString("\n")

	for _, c := range channels {
		b.WriteString(renderChannelRow(s, c))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderChannelRow(s *styles, c htmlChannel) string {
	labelStyle := s.chLabel
	if !c.HasData {
		labelStyle = s.chLabelMuted
	}
	label := labelStyle.Render(fmt.Sprintf("%-*s", chLabelW, trimName(c.Label, chLabelW)))

	if !c.HasData {
		bar := s.barTrack.Render(strings.Repeat(" ", chBarW))
		score := s.chConf.Render(fmt.Sprintf("%*s", chPctW, "—"))
		conf := s.chConf.Render(fmt.Sprintf("%-*s", chConfW, ""))
		zone := s.zoneNone.Render(fmt.Sprintf("%-*s", chZoneW, "—"))
		return label + "  " + bar + "  " + score + "  " + conf + "  " + zone
	}

	bar := renderBar(s, c.ScoreBar, chBarW, c.ZoneClass)
	score := s.chScore.Render(fmt.Sprintf("%*s", chPctW, c.ScorePct))
	conf := s.chConf.Render(fmt.Sprintf("conf %-*s", chConfW-5, c.ConfPct))
	zoneText := c.Zone
	if len(zoneText) > chZoneW {
		zoneText = zoneText[:chZoneW]
	}
	zone := s.zoneStyle(c.ZoneClass).Render(fmt.Sprintf("%-*s", chZoneW, zoneText))
	return label + "  " + bar + "  " + score + "  " + conf + "  " + zone
}

func renderBoostsStrip(s *styles, boosts []htmlMetric, innerWidth int) string {
	var b strings.Builder
	b.WriteString(s.subhead.Render("BOOSTS & OVERRIDES"))
	b.WriteString("\n")

	chips := make([]string, 0, len(boosts))
	for _, m := range boosts {
		chips = append(chips, boostChip(s, m))
	}
	b.WriteString(joinPillsWrap(s, chips, innerWidth))
	return b.String()
}

func boostChip(s *styles, m htmlMetric) string {
	label := s.boostLabel.Render(strings.ToUpper(m.Label))
	val := s.boostStyle(m.Class).Render(m.Value)
	inner := label + " " + val
	return s.r.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorLine).
		Padding(0, 1).
		Render(inner)
}

func renderCategoriesGrid(s *styles, cats []htmlCategory, innerWidth int) string {
	cols := 2
	if innerWidth < 60 {
		cols = 1
	}
	if innerWidth >= 110 {
		cols = 3
	}
	colWidth := (innerWidth - (cols-1)*2) / cols

	columns := make([][]string, cols)
	for i := range columns {
		columns[i] = make([]string, 0)
	}
	for i, cat := range cats {
		columns[i%cols] = append(columns[i%cols], renderCategoryBlock(s, cat, colWidth))
	}

	colStrs := make([]string, cols)
	for i, blocks := range columns {
		colStrs[i] = lipgloss.JoinVertical(lipgloss.Left, blocks...)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, interleaveGap(colStrs, "  ")...)
}

func renderCategoryBlock(s *styles, cat htmlCategory, width int) string {
	var b strings.Builder
	title := s.categoryTitle.Render(strings.ToUpper(cat.Title))
	if cat.Note != "" {
		title = title + " " + s.categoryNote.Render(cat.Note)
	}
	b.WriteString(title + "\n")

	labelW := width - 10
	if labelW < 12 {
		labelW = 12
	}
	for _, m := range cat.Metrics {
		valStr := m.Value
		valW := lipgloss.Width(valStr)
		if valW > 10 {
			valW = 10
		}
		labelCol := s.metricLabel.Render(trimName(m.Label, labelW))
		// Right-align the value within (width - labelW - 1) cells using a
		// padded raw string, then style the trimmed value.
		availForVal := width - lipgloss.Width(labelCol) - 1
		if availForVal < 4 {
			availForVal = 4
		}
		pad := availForVal - valW
		if pad < 1 {
			pad = 1
		}
		spacer := strings.Repeat(" ", pad)
		val := s.metricStyle(m.Class).Render(valStr)
		b.WriteString(labelCol + spacer + val + "\n")
	}
	b.WriteString("\n")
	return strings.TrimRight(b.String(), "\n")
}
