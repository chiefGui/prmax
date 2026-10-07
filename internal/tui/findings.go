package tui

import (
	"fmt"
	"path"
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

func shortLoc(f model.Finding) string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", path.Base(f.File), f.Line)
	}
	return path.Base(f.File)
}

func renderFindings(r model.Review, w, sel int) (string, int) {
	var b strings.Builder
	lines := 0
	write := func(s string) {
		b.WriteString(s + "\n")
		lines += strings.Count(s, "\n") + 1
	}
	if r.Status == model.StatusReviewing {
		write(" " + sTitle.Render("Review in progress. Press tab for the live log."))
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
		write(" " + sGreen.Render("No findings."))
		return b.String(), 0
	}
	bodyW := min(w-8, measure)
	rowW := min(w-2, measure+6)
	selLine, i := 0, 0
	for _, c := range model.Categories {
		fs := r.ByCategory(c.ID)
		if len(fs) == 0 {
			continue
		}
		write(" " + sBold.Render(c.Label) + "  " + sDim.Render(fmt.Sprint(len(fs))))
		write("")
		for _, f := range fs {
			i++
			on := i-1 == sel
			mark, num, title := "  ", sFaint.Render(fmt.Sprintf("%-2d", i)), sText.Render(strings.TrimSpace(f.Title))
			if on {
				selLine = lines
				mark, num, title = sTitle.Render("▍")+" ", sDim.Render(fmt.Sprintf("%-2d", i)), sBold.Render(strings.TrimSpace(f.Title))
			}
			if f.StillOpen {
				title += "  " + sOrange.Render("still open")
			}
			loc := sDim.Render(shortLoc(f))
			gap := max(2, rowW-4-lipgloss.Width(num)-lipgloss.Width(title)-lipgloss.Width(loc))
			write(" " + mark + num + " " + title + strings.Repeat(" ", gap) + loc)
			if on {
				body := lipgloss.NewStyle().Width(bodyW).Render(strings.TrimSpace(f.Text))
				write(indent(sText.Render(body), "      "))
				write("      " + sFaint.Render(location(f)))
				write("")
			}
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

func (m Model) roundsBar(p model.PR) string {
	var t []string
	for i, name := range []string{"Findings", "Log", "Prompt"} {
		if i == m.tab {
			t = append(t, sTabOn.Render(name))
		} else {
			t = append(t, sTab.Render(name))
		}
	}
	left := "  " + strings.Join(t, "   ")
	r := m.currentReview()
	if r == nil {
		return "\n" + left + "\n\n"
	}
	head := []string{}
	if r.Round > 0 {
		head = append(head, fmt.Sprintf("Round %d", r.Round))
	}
	if r.ModelLabel != "" {
		head = append(head, r.ModelLabel)
	}
	if len(p.Reviews) > 1 {
		head = append(head, fmt.Sprintf("%d/%d", m.reviewIdx+1, len(p.Reviews)))
	}
	right := sText.Render(strings.Join(head, " · "))
	gap := max(2, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)

	var details []string
	if r.SinceSHA != "" {
		details = append(details, "since "+short(r.SinceSHA))
	}
	if r.Access != "" {
		details = append(details, r.Access+" access")
	}
	details = append(details, r.StartedAt.Local().Format("Jan 2 15:04"))
	if !r.FinishedAt.IsZero() {
		details = append(details, r.FinishedAt.Sub(r.StartedAt).Round(time.Second).String())
	}
	line := sDim.Render(strings.Join(details, " · "))
	if r.CommentURL != "" {
		line += sDim.Render(" · ") + sGreen.Render("✓") + sDim.Render(" posted")
	}
	switch {
	case r.NudgeError != "":
		line += sDim.Render(" · ") + sRed.Render("nudge failed: "+clip(r.NudgeError, 60))
	case !r.NudgedAt.IsZero():
		line += sDim.Render(" · ") + sGreen.Render("✓") + sDim.Render(" nudged "+r.NudgedAt.Local().Format("15:04"))
	}
	return "\n" + left + strings.Repeat(" ", gap) + right + "\n  " + line + "\n"
}
