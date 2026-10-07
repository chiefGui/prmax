package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"prmax/internal/model"
)

const measure = 80

func ordered(r model.Review) []model.Finding {
	var out []model.Finding
	for _, c := range model.Categories {
		out = append(out, r.ByCategory(c.ID)...)
	}
	return out
}

func findingURL(r model.Review, f model.Finding) string {
	url := fmt.Sprintf("https://github.com/%s/blob/%s/%s", r.Repo, r.HeadSHA, f.File)
	if f.Line > 0 {
		url += fmt.Sprintf("#L%d", f.Line)
	}
	return url
}

func renderFindings(r model.Review, w, sel int) (string, int) {
	var b strings.Builder
	lines := 0
	write := func(s string) {
		b.WriteString(s + "\n")
		lines += strings.Count(s, "\n") + 1
	}
	write("")
	if r.Status == model.StatusReviewing || r.Status == model.StatusQueued {
		write(" " + sDim.Render("In progress. Press tab for the live log."))
		return b.String(), 0
	}
	if r.Error != "" {
		write(" " + sRed.Render(lipgloss.NewStyle().Width(min(w-4, measure)).Render(r.Error)))
		write("")
	}
	if r.Status != model.StatusClean && r.Status != model.StatusFindings {
		return b.String(), 0
	}
	if len(r.Findings) == 0 {
		write(" " + sGreen.Render("✓ No findings."))
		return b.String(), 0
	}
	bodyW := min(w-8, measure)
	selLine, i := 0, 0
	for _, c := range model.Categories {
		fs := r.ByCategory(c.ID)
		if len(fs) == 0 {
			continue
		}
		write(" " + sDim.Render(strings.ToUpper(c.Label)) + "  " + sFaint.Render(fmt.Sprint(len(fs))))
		for _, f := range fs {
			i++
			title := clip(strings.TrimSpace(f.Title), max(10, bodyW))
			if i-1 != sel {
				line := " " + sFaint.Render(fmt.Sprintf("%3d", i)) + "  " + sText.Render(title)
				if f.StillOpen {
					line += "  " + sOrange.Render("still open")
				}
				write(line)
				continue
			}
			selLine = lines
			line := sTitle.Render("▍") + sDim.Render(fmt.Sprintf("%3d", i)) + "  " + sBold.Render(title)
			if f.StillOpen {
				line += "  " + sOrange.Render("still open")
			}
			write(line)
			body := lipgloss.NewStyle().Width(bodyW).Render(strings.TrimSpace(f.Text))
			write(indent(sText.Render(body), "      "))
			write("      " + sTitle.Render(location(f)))
			write("")
		}
		write("")
	}
	return b.String(), selLine
}

func location(f model.Finding) string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return f.File
}

func (m Model) roundMark(r model.Review) string {
	switch r.Status {
	case model.StatusClean:
		return sGreen.Render("✓")
	case model.StatusFindings:
		return sOrange.Render(fmt.Sprintf("△ %d", len(r.Findings)))
	case model.StatusFailed:
		return sRed.Render("✕")
	case model.StatusReviewing, model.StatusQueued:
		return m.spin.View()
	}
	return sFaint.Render("–")
}

func (m Model) roundsBar(p model.PR) string {
	var rounds []string
	for i, r := range p.Reviews {
		n := r.Round
		if n == 0 {
			n = i + 1
		}
		name := sTab.Render(fmt.Sprintf("Round %d", n))
		if i == m.reviewIdx {
			name = sTabOn.Render(fmt.Sprintf("Round %d", n))
		}
		rounds = append(rounds, name+" "+m.roundMark(r))
	}
	left := "  " + strings.Join(rounds, "    ")
	if len(rounds) == 0 {
		left = "  " + sDim.Render("Not reviewed yet. Press r to review.")
	}
	var views []string
	for i, name := range []string{"Findings", "Log"} {
		if i == m.tab {
			views = append(views, sTabOn.Render(name))
		} else {
			views = append(views, sTab.Render(name))
		}
	}
	right := strings.Join(views, "  ")
	gap := max(2, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	top := left + strings.Repeat(" ", gap) + right

	r := m.currentReview()
	if r == nil {
		return top + "\n"
	}
	sep := sFaint.Render(" · ")
	var details []string
	if r.ModelLabel != "" {
		details = append(details, sText.Render(r.ModelLabel))
	}
	if r.Access != "" {
		details = append(details, sDim.Render(r.Access+" access"))
	}
	if r.SinceSHA != "" {
		details = append(details, sDim.Render("since "+short(r.SinceSHA)))
		if r.Scope == model.ScopeWhole {
			details = append(details, sDim.Render("whole PR"))
		}
	}
	details = append(details, sDim.Render(r.StartedAt.Local().Format("Jan 2 15:04")))
	if !r.FinishedAt.IsZero() {
		details = append(details, sDim.Render(r.FinishedAt.Sub(r.StartedAt).Round(time.Second).String()))
	}
	if r.CommentURL != "" {
		details = append(details, sGreen.Render("✓")+sDim.Render(" posted"))
	} else if r.Status == model.StatusClean || r.Status == model.StatusFindings {
		details = append(details, sRed.Render("not posted"))
	}
	switch {
	case r.NudgeError != "":
		details = append(details, sRed.Render("nudge failed: "+clip(r.NudgeError, 60)))
	case !r.NudgedAt.IsZero():
		details = append(details, sGreen.Render("✓")+sDim.Render(" nudged "+r.NudgedAt.Local().Format("15:04")))
	}
	return top + "\n  " + strings.Join(details, sep)
}
