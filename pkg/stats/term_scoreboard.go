package stats

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderScoreboard renders T and CT (or "All") side-by-side. teams comes
// from buildScoreboard() in html_reporter.go and is already sorted by
// kills desc. Returns an empty string if there is nothing to render.
func renderScoreboard(s *styles, teams []htmlTeam) string {
	if len(teams) == 0 {
		return ""
	}

	blocks := make([]string, 0, len(teams))
	for _, t := range teams {
		blocks = append(blocks, renderTeamTable(s, t))
	}

	gap := s.r.NewStyle().Padding(0, 2).Render("")
	joined := lipgloss.JoinHorizontal(lipgloss.Top, interleaveGap(blocks, gap)...)
	return joined
}

func interleaveGap(blocks []string, gap string) []string {
	if len(blocks) <= 1 {
		return blocks
	}
	out := make([]string, 0, len(blocks)*2-1)
	for i, b := range blocks {
		if i > 0 {
			out = append(out, gap)
		}
		out = append(out, b)
	}
	return out
}

// Fixed column widths chosen to fit a typical 80-col terminal even with two
// teams placed side-by-side. Tabular numbers keep things lined up.
const (
	colName    = 16
	colNarrow  = 4
	colADR     = 5
	colHS      = 5
	colMVP     = 4
	teamMinTot = colName + colNarrow*3 + colADR + colHS + colMVP + 6 // padding
)

func renderTeamTable(s *styles, t htmlTeam) string {
	var b strings.Builder

	label := s.teamLabelCT.Render(t.Label)
	if t.Label == "T" {
		label = s.teamLabelT.Render("T")
	} else if t.Label == "All" {
		label = s.subhead.Render("All")
	}
	plural := "s"
	if len(t.Players) == 1 {
		plural = ""
	}
	count := s.teamCount.Render(fmt.Sprintf("  %d player%s", len(t.Players), plural))
	b.WriteString(label + count + "\n")

	header := s.tableHeader.Render(fmt.Sprintf(
		"%-*s %*s %*s %*s %*s %*s %*s",
		colName, "Player",
		colNarrow, "K",
		colNarrow, "D",
		colNarrow, "A",
		colADR, "ADR",
		colHS, "HS%",
		colMVP, "MVP",
	))
	b.WriteString(header + "\n")

	for _, row := range t.Players {
		name := trimName(row.Name, colName)
		nameCell := s.tableName.Render(fmt.Sprintf("%-*s", colName, name))

		k := s.tableNum.Render(fmt.Sprintf("%*s", colNarrow, row.Kills))
		d := s.tableNum.Render(fmt.Sprintf("%*s", colNarrow, row.Deaths))
		a := s.tableNum.Render(fmt.Sprintf("%*s", colNarrow, row.Assists))

		adr := numOrMuted(s, row.ADR, colADR)
		hs := numOrMuted(s, row.HS, colHS)
		mvp := numOrMuted(s, row.MVPs, colMVP)

		b.WriteString(nameCell + " " + k + " " + d + " " + a + " " + adr + " " + hs + " " + mvp + "\n")
	}

	return b.String()
}

func numOrMuted(s *styles, v string, width int) string {
	if v == "—" || v == "0" {
		return s.tableMuted.Render(fmt.Sprintf("%*s", width, v))
	}
	return s.tableNum.Render(fmt.Sprintf("%*s", width, v))
}

func trimName(name string, width int) string {
	if len(name) <= width {
		return name
	}
	if width <= 1 {
		return name[:width]
	}
	return name[:width-1] + "…"
}
