package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"prmax/internal/model"
)

const sidebarW = 22

type filterDef struct {
	name  string
	match func(model.PR) bool
}

var filters = []filterDef{
	{"All", func(model.PR) bool { return true }},
	{"To review", func(p model.PR) bool {
		return p.Status == model.StatusIdle || p.Status == model.StatusOutdated || p.Status == model.StatusFailed
	}},
	{"Reviewing", func(p model.PR) bool {
		return p.Status == model.StatusReviewing || p.Status == model.StatusQueued
	}},
	{"Has issues", func(p model.PR) bool { return p.Status == model.StatusFindings }},
	{"Passed", func(p model.PR) bool { return p.Status == model.StatusClean }},
}

func (m Model) repos() []string {
	var out []string
	for _, f := range m.state.Forwarders {
		out = append(out, f.Repo)
	}
	return out
}

func (m Model) repoFilter() string {
	repos := m.repos()
	if m.repoIdx <= 0 || m.repoIdx > len(repos) {
		return ""
	}
	return repos[m.repoIdx-1]
}

func (m Model) count(filter int, extra func(model.PR) bool) int {
	repo := m.repoFilter()
	n := 0
	for _, p := range m.state.PRs {
		if repo != "" && p.Repo != repo {
			continue
		}
		if filter >= 0 && !filters[filter].match(p) {
			continue
		}
		if extra != nil && !extra(p) {
			continue
		}
		n++
	}
	return n
}

func (m Model) visible() []model.PR {
	repo := m.repoFilter()
	f := filters[m.filter]
	var out []model.PR
	for _, p := range m.state.PRs {
		if (repo == "" || p.Repo == repo) && f.match(p) {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) cursorPR() (model.PR, bool) {
	v := m.visible()
	if m.cursor < 0 || m.cursor >= len(v) {
		return model.PR{}, false
	}
	return v[m.cursor], true
}

func (m Model) bodyRows() int { return max(3, m.height-4) }

func (m Model) listRows() int { return m.bodyRows() - 2 }

func (m Model) listStart() int {
	if rows := m.listRows(); m.cursor >= rows {
		return m.cursor - rows + 1
	}
	return 0
}

func (m Model) setRepo(i int) Model {
	n := len(m.repos())
	m.repoIdx = (i%(n+1) + n + 1) % (n + 1)
	m.cursor = 0
	return m
}

func (m Model) setFilter(i int) Model {
	m.filter = (i%len(filters) + len(filters)) % len(filters)
	m.cursor = 0
	return m
}

func (m Model) keyList(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.visible())
	switch s := k.String(); s {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(n-1, m.cursor+1)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(0, n-1)
	case "left", "h":
		return m.setRepo(m.repoIdx - 1), nil
	case "right", "l":
		return m.setRepo(m.repoIdx + 1), nil
	case "tab":
		return m.setFilter(m.filter + 1), nil
	case "shift+tab":
		return m.setFilter(m.filter - 1), nil
	case "1", "2", "3", "4", "5":
		return m.setFilter(int(s[0] - '1')), nil
	case "enter":
		if p, ok := m.cursorPR(); ok {
			return m.openDetail(p)
		}
	default:
		if p, ok := m.cursorPR(); ok {
			return m, m.prAction(p, s)
		}
	}
	return m, nil
}

func (m Model) mouseList(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	n := len(m.visible())
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.cursor = max(0, m.cursor-1)
	case tea.MouseButtonWheelDown:
		m.cursor = min(n-1, m.cursor+1)
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionRelease {
			return m, nil
		}
		if msg.X < sidebarW {
			if i := msg.Y - 3; i >= 0 && i <= len(m.repos()) {
				return m.setRepo(i), nil
			}
			return m, nil
		}
		if msg.Y == 2 {
			x := sidebarW + 3
			for i := range filters {
				w := lipgloss.Width(m.filterTab(i))
				if msg.X >= x-2 && msg.X < x+w+2 {
					return m.setFilter(i), nil
				}
				x += w + 4
			}
			return m, nil
		}
		row := msg.Y - 4
		if row < 0 || row >= m.listRows() {
			return m, nil
		}
		i := m.listStart() + row
		if i >= n {
			return m, nil
		}
		if i == m.cursor {
			return m.openDetail(m.visible()[i])
		}
		m.cursor = i
	}
	return m, nil
}

func (m Model) filterTab(i int) string {
	n := m.count(i, nil)
	if i == m.filter {
		return sTabOn.Render(filters[i].name) + " " + sDim.Render(fmt.Sprint(n))
	}
	return sTab.Render(filters[i].name) + " " + sFaintNum(n)
}

func sFaintNum(n int) string {
	if n == 0 {
		return sFaint.Render("0")
	}
	return sDim.Render(fmt.Sprint(n))
}

func (m Model) sidebar() []string {
	lines := []string{""}
	item := func(i int, label string, down bool, count int) string {
		mark := "  "
		name := sDim.Render(clip(label, sidebarW-9))
		if i == m.repoIdx {
			mark = sTitle.Render("▍") + " "
			name = sBold.Render(clip(label, sidebarW-9))
		}
		if down {
			name += " " + sRed.Render("•")
		}
		num := sFaintNum(count)
		gap := max(1, sidebarW-2-lipgloss.Width(mark)-lipgloss.Width(name)-lipgloss.Width(num))
		return " " + mark + name + strings.Repeat(" ", gap) + num + " "
	}
	lines = append(lines, item(0, "All repos", false, len(m.state.PRs)))
	for i, f := range m.state.Forwarders {
		c := 0
		for _, p := range m.state.PRs {
			if p.Repo == f.Repo {
				c++
			}
		}
		lines = append(lines, item(i+1, shortRepo(f.Repo), !f.Connected, c))
	}
	return lines
}

func (m Model) viewList() string {
	var b strings.Builder
	b.WriteString(m.header() + "\n")
	b.WriteString(rule(m.width) + "\n")

	rightW := max(20, m.width-sidebarW-1)
	var tabs []string
	for i := range filters {
		tabs = append(tabs, m.filterTab(i))
	}
	right := []string{"  " + strings.Join(tabs, "    "), ""}
	v := m.visible()
	if len(v) == 0 {
		right = append(right, "  "+sDim.Render("Nothing here."))
	} else {
		showRepo := m.repoFilter() == ""
		repoW := 0
		if showRepo {
			for _, p := range v {
				repoW = max(repoW, len(shortRepo(p.Repo)))
			}
		}
		start := m.listStart()
		for i := start; i < len(v) && i-start < m.listRows(); i++ {
			right = append(right, m.row(v[i], repoW, rightW, i == m.cursor))
		}
	}

	side := m.sidebar()
	bar := sFaint.Render("│")
	for i := 0; i < m.bodyRows(); i++ {
		l := ""
		if i < len(side) {
			l = side[i]
		}
		l += strings.Repeat(" ", max(0, sidebarW-lipgloss.Width(l)))
		r := ""
		if i < len(right) {
			r = right[i]
		}
		b.WriteString(l + bar + r + "\n")
	}
	b.WriteString(rule(m.width) + "\n")
	b.WriteString(m.footer(keys("enter", "open", "r", "review", "tab", "filter", "←→", "repo", "o", "browser", "q", "quit")))
	return b.String()
}

func (m Model) row(p model.PR, repoW, width int, sel bool) string {
	icon, info := m.statusBits(p)
	repo := ""
	if repoW > 0 {
		repo = sDim.Render(fmt.Sprintf("%-*s", repoW, shortRepo(p.Repo))) + "   "
		repoW += 3
	}
	num := sDim.Render(fmt.Sprintf("#%-4d", p.Number))
	age := sFaint.Render(fmt.Sprintf("%4s", ago(p.UpdatedAt)))
	infoW := 14
	fixed := 2 + 1 + 3 + repoW + 5 + 3 + 3 + infoW + 2 + 4 + 2
	titleW := max(10, width-fixed)
	titleStyle := sText
	if sel {
		titleStyle = sBold
	}
	title := titleStyle.Render(fmt.Sprintf("%-*s", titleW, clip(p.Title, titleW)))
	info += strings.Repeat(" ", max(0, infoW-lipgloss.Width(info)))
	line := fmt.Sprintf("  %s   %s%s   %s   %s  %s  ", icon, repo, num, title, info, age)
	if sel {
		return sSelected.Render(line + strings.Repeat(" ", max(0, width-lipgloss.Width(line))))
	}
	return line
}
