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
	pr    model.PR
	ready bool
	list  []model.Provider
	pick  map[string]string
	effs  map[string]string
	prov  int
	mdl   int
	eff   int
	field int
}

func (c *confirmState) init(p model.Providers) {
	c.list = p.List
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
	return m, m.loadProviders()
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
	case "down", "j", "tab":
		c.field = min(2, c.field+1)
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
	b.WriteString(sDim.Render("Review") + "  " + sBold.Render(fmt.Sprintf("%s #%d", shortRepo(c.pr.Repo), c.pr.Number)) + "\n")
	b.WriteString(sDim.Render(clip(c.pr.Title, inner)) + "\n\n")
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
