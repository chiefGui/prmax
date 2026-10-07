package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"prmax/internal/model"
)

type openConfirmMsg struct{ pr model.PR }

type providersMsg struct{ p model.Providers }

type confirmState struct {
	pr     model.PR
	ready  bool
	list   []model.Provider
	pick   map[string]string
	effs   map[string]string
	prov   int
	mdl    int
	eff    int
	field  int
	plan   *model.Plan
	err    string
	access string
	nudge  bool
	scope  string
}

type planMsg struct {
	key  string
	plan model.Plan
	err  error
}

func (c *confirmState) init(p model.Providers) {
	c.list = p.List
	c.access = p.Choice.Access
	c.nudge = p.Choice.Nudge
	c.scope = p.Choice.Scope
	c.pick = map[string]string{}
	for k, v := range p.Choice.Models {
		c.pick[k] = v
	}
	c.effs = map[string]string{}
	for k, v := range p.Choice.Efforts {
		c.effs[k] = v
	}
	c.prov = 0
	for i, pv := range c.list {
		if pv.ID == p.Choice.Provider {
			c.prov = i
		}
	}
	c.syncModel()
	c.ready = len(c.list) > 0
}

func (c *confirmState) syncModel() {
	if len(c.list) == 0 {
		return
	}
	pv := c.list[c.prov]
	want := c.pick[pv.ID]
	if want == "" {
		want = pv.Default
	}
	c.mdl = 0
	for i, m := range pv.Models {
		if m.ID == want {
			c.mdl = i
		}
	}
	c.syncEffort()
}

func (c *confirmState) efforts() []string {
	pv := c.list[c.prov]
	if len(pv.Models) == 0 {
		return nil
	}
	return pv.Models[c.mdl].Efforts
}

func (c *confirmState) syncEffort() {
	pv := c.list[c.prov]
	if len(pv.Models) == 0 {
		return
	}
	mo := pv.Models[c.mdl]
	c.eff = 0
	for _, want := range []string{c.effs[pv.ID], mo.DefaultEffort} {
		for i, e := range mo.Efforts {
			if e == want && want != "" {
				c.eff = i
				return
			}
		}
	}
}

func (c *confirmState) request() (model.ReviewRequest, bool) {
	if !c.ready || len(c.list[c.prov].Models) == 0 {
		return model.ReviewRequest{}, false
	}
	pv := c.list[c.prov]
	req := model.ReviewRequest{Provider: pv.ID, Model: pv.Models[c.mdl].ID}
	if effs := c.efforts(); c.eff < len(effs) {
		req.Effort = effs[c.eff]
	}
	req.Access = model.AccessModes[c.accessIdx()]
	req.Nudge = c.nudge
	req.Scope = model.Scopes[c.scopeIdx()]
	return req, true
}

func (m Model) loadProviders() tea.Cmd {
	return func() tea.Msg {
		p, err := m.client.Providers()
		if err != nil {
			return nil
		}
		return providersMsg{p: p}
	}
}

func (m Model) openConfirm(p model.PR) (tea.Model, tea.Cmd) {
	c := &confirmState{pr: p}
	if m.providers != nil {
		c.init(*m.providers)
	}
	m.confirm = c
	return m, tea.Batch(m.loadProviders(), m.loadPlan(p))
}

func (m Model) loadPlan(p model.PR) tea.Cmd {
	return func() tea.Msg {
		pl, err := m.client.Plan(p.Repo, p.Number)
		return planMsg{key: p.Key(), plan: pl, err: err}
	}
}

func (c *confirmState) planLines(width int) []string {
	switch {
	case c.err != "":
		return []string{sRed.Render(clip(c.err, width))}
	case c.plan == nil:
		return []string{sDim.Render("Checking what changed…")}
	}
	pl := c.plan
	issues := fmt.Sprintf("%d issues", pl.SinceIssues)
	if pl.SinceIssues == 1 {
		issues = "1 issue"
	}
	if pl.Unchanged {
		head := sDim.Render(fmt.Sprintf("Round %d · nothing changed since round %d (%s)", pl.Round, pl.SinceRound, issues))
		if c.whole() {
			return []string{head, "", sText.Render("Reviews the whole PR again.")}
		}
		return []string{head, "", sOrange.Render("Nothing to review. Set scope to whole PR to review it again.")}
	}
	if pl.Since == "" {
		note := "full PR"
		if pl.Rewritten {
			note = "full PR · history was rewritten since the last review"
		}
		return []string{sDim.Render(fmt.Sprintf("Round %d · %s", pl.Round, note))}
	}
	lines := []string{sDim.Render(fmt.Sprintf("Round %d · since %s (round %d: %s)", pl.Round, short(pl.Since), pl.SinceRound, issues)), ""}
	const maxShown = 6
	for i, cm := range pl.Commits {
		if i == maxShown {
			lines = append(lines, sDim.Render(fmt.Sprintf("  … %d more", len(pl.Commits)-maxShown)))
			break
		}
		lines = append(lines, "  "+sTitle.Render(short(cm.SHA))+"  "+sText.Render(clip(cm.Title, width-11)))
	}
	commits := fmt.Sprintf("%d commits", len(pl.Commits))
	if len(pl.Commits) == 1 {
		commits = "this commit"
	} else if len(pl.Commits) > 1 {
		commits = "these " + commits
	}
	summary := "Reviews " + commits + " against the rest of the PR"
	if c.whole() {
		summary = "Reviews the whole PR, starting with " + commits
	}
	if pl.SinceIssues > 0 {
		summary += fmt.Sprintf(" and re-checks the %s.", issues)
	} else {
		summary += "."
	}
	return append(lines, "", sText.Render(summary))
}

func (m Model) keyConfirm(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.confirm
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.confirm = nil
		return m, nil
	case "up", "k", "shift+tab":
		c.field = max(0, c.field-1)
		if c.field == 4 && !c.rereview() {
			c.field = 3
		}
	case "down", "j", "tab":
		c.field = min(5, c.field+1)
		if c.field == 4 && !c.rereview() {
			c.field = 5
		}
	case "left", "h":
		c.move(-1)
	case "right", "l":
		c.move(1)
	case "enter", "r", " ":
		req, ok := c.request()
		if !ok {
			return m, nil
		}
		pr := c.pr
		m.confirm = nil
		return m, func() tea.Msg {
			if err := m.client.Review(pr.Repo, pr.Number, req); err != nil {
				return flashMsg("error: " + err.Error())
			}
			return flashMsg(fmt.Sprintf("review queued for #%d", pr.Number))
		}
	}
	return m, nil
}

func (c *confirmState) move(d int) {
	if !c.ready {
		return
	}
	if c.field == 0 {
		c.prov = (c.prov + d + len(c.list)) % len(c.list)
		c.syncModel()
		return
	}
	if c.field == 5 {
		c.nudge = !c.nudge
		return
	}
	if c.field == 4 {
		c.scope = model.Scopes[(c.scopeIdx()+1)%len(model.Scopes)]
		return
	}
	if c.field == 3 {
		n := len(model.AccessModes)
		c.access = model.AccessModes[(c.accessIdx()+d+n)%n]
		return
	}
	if c.field == 2 {
		effs := c.efforts()
		if len(effs) == 0 {
			return
		}
		c.eff = (c.eff + d + len(effs)) % len(effs)
		c.effs[c.list[c.prov].ID] = effs[c.eff]
		return
	}
	n := len(c.list[c.prov].Models)
	if n == 0 {
		return
	}
	c.mdl = (c.mdl + d + n) % n
	c.pick[c.list[c.prov].ID] = c.list[c.prov].Models[c.mdl].ID
	c.syncEffort()
}

func (m Model) viewConfirm() string {
	c := m.confirm
	boxW := min(84, max(40, m.width-8))
	inner := boxW - 6
	var b strings.Builder
	verb := "Review"
	if c.plan != nil && c.plan.Since != "" {
		verb = "Re-review"
	}
	b.WriteString(sDim.Render(verb) + "  " + sBold.Render(fmt.Sprintf("%s #%d", shortRepo(c.pr.Repo), c.pr.Number)) + "\n")
	b.WriteString(sDim.Render(clip(c.pr.Title, inner)) + "\n\n")
	for _, l := range c.planLines(inner) {
		b.WriteString(l + "\n")
	}
	b.WriteString("\n")
	if !c.ready {
		b.WriteString(sDim.Render("Loading providers…") + "\n")
	} else {
		var provs, mdls []string
		for _, p := range c.list {
			provs = append(provs, p.Label)
		}
		for _, mo := range c.list[c.prov].Models {
			mdls = append(mdls, mo.Label)
		}
		b.WriteString(optionRow("Provider", provs, c.prov, c.field == 0, inner) + "\n")
		b.WriteString(optionRow("Model", mdls, c.mdl, c.field == 1, inner) + "\n")
		b.WriteString(optionRow("Effort", c.efforts(), c.eff, c.field == 2, inner) + "\n")
		b.WriteString(optionRow("Access", model.AccessModes, c.accessIdx(), c.field == 3, inner) + "\n")
		if c.rereview() {
			b.WriteString(optionRow("Scope", model.Scopes, c.scopeIdx(), c.field == 4, inner) + "\n")
		}
		b.WriteString(optionRow("Nudge", nudgeModes, c.nudgeIdx(), c.field == 5, inner) + "\n")
	}
	b.WriteString("\n" + keys("enter", "review", "←→", "change", "↑↓", "field", "esc", "cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cFaint).
		Padding(1, 2).
		Width(boxW).
		Render(b.String())

	var out strings.Builder
	out.WriteString(m.header() + "\n")
	out.WriteString(rule(m.width) + "\n")
	out.WriteString(lipgloss.Place(m.width, max(1, m.height-4), lipgloss.Center, lipgloss.Center, box) + "\n")
	out.WriteString(rule(m.width) + "\n")
	out.WriteString(m.footer(""))
	return out.String()
}

func optionRow(label string, opts []string, sel int, focused bool, width int) string {
	mark := "  "
	lab := sDim.Render(fmt.Sprintf("%-10s", label))
	if focused {
		mark = sTitle.Render("▍") + " "
		lab = sText.Render(fmt.Sprintf("%-10s", label))
	}
	avail := width - 12
	render := func(i int) string {
		if i == sel {
			if focused {
				return sTabOn.Foreground(cAccent).Render(opts[i])
			}
			return sBold.Render(opts[i])
		}
		return sDim.Render(opts[i])
	}
	if len(opts) == 0 {
		return mark + lab + sDim.Render("none available")
	}
	lo, hi := sel, sel
	used := lipgloss.Width(opts[sel])
	for {
		grew := false
		if hi+1 < len(opts) && used+3+lipgloss.Width(opts[hi+1])+4 <= avail {
			hi++
			used += 3 + lipgloss.Width(opts[hi])
			grew = true
		}
		if lo-1 >= 0 && used+3+lipgloss.Width(opts[lo-1])+4 <= avail {
			lo--
			used += 3 + lipgloss.Width(opts[lo])
			grew = true
		}
		if !grew {
			break
		}
	}
	var parts []string
	for i := lo; i <= hi; i++ {
		parts = append(parts, render(i))
	}
	row := strings.Join(parts, "   ")
	if lo > 0 {
		row = sFaint.Render("‹ ") + row
	}
	if hi < len(opts)-1 {
		row += sFaint.Render(" ›")
	}
	return mark + lab + row
}

func (c *confirmState) accessIdx() int {
	for i, a := range model.AccessModes {
		if a == c.access {
			return i
		}
	}
	return 0
}

var nudgeModes = []string{"off", "after posting"}

func (c *confirmState) nudgeIdx() int {
	if c.nudge {
		return 1
	}
	return 0
}

func (c *confirmState) scopeIdx() int {
	for i, s := range model.Scopes {
		if s == c.scope {
			return i
		}
	}
	return 0
}

func (c *confirmState) whole() bool { return model.Scopes[c.scopeIdx()] == model.ScopeWhole }

func (c *confirmState) rereview() bool {
	return c.plan != nil && (c.plan.Since != "" || c.plan.Unchanged)
}
