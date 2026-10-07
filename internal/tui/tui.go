package tui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"prmax/internal/model"
)

var (
	cText   = lipgloss.AdaptiveColor{Light: "#2E2E33", Dark: "#D6D6DC"}
	cMuted  = lipgloss.AdaptiveColor{Light: "#8C8C96", Dark: "#73737D"}
	cFaint  = lipgloss.AdaptiveColor{Light: "#D9D9DE", Dark: "#2F2F36"}
	cAccent = lipgloss.AdaptiveColor{Light: "#5A67C8", Dark: "#A3ACEB"}
	cGreen  = lipgloss.AdaptiveColor{Light: "#3D8A5A", Dark: "#8CC79F"}
	cAmber  = lipgloss.AdaptiveColor{Light: "#A8742A", Dark: "#D9B877"}
	cRed    = lipgloss.AdaptiveColor{Light: "#BF4F48", Dark: "#E3958D"}
	cViolet = lipgloss.AdaptiveColor{Light: "#8456C4", Dark: "#C4A7F0"}
	cTeal   = lipgloss.AdaptiveColor{Light: "#2E8A8A", Dark: "#86CFCB"}
	cSelBg  = lipgloss.AdaptiveColor{Light: "#EEEFF6", Dark: "#24252C"}

	sTitle    = lipgloss.NewStyle().Foreground(cAccent)
	sDim      = lipgloss.NewStyle().Foreground(cMuted)
	sFaint    = lipgloss.NewStyle().Foreground(cFaint)
	sText     = lipgloss.NewStyle().Foreground(cText)
	sBold     = lipgloss.NewStyle().Bold(true).Foreground(cText)
	sGreen    = lipgloss.NewStyle().Foreground(cGreen)
	sOrange   = lipgloss.NewStyle().Foreground(cAmber)
	sRed      = lipgloss.NewStyle().Foreground(cRed)
	sViolet   = lipgloss.NewStyle().Foreground(cViolet)
	sTeal     = lipgloss.NewStyle().Foreground(cTeal)
	sSelected = lipgloss.NewStyle().Background(cSelBg)
	sTab      = lipgloss.NewStyle().Foreground(cMuted)
	sTabOn    = lipgloss.NewStyle().Foreground(cText).Bold(true).Underline(true)
)

func rule(w int) string { return sFaint.Render(strings.Repeat("─", w)) }

func keys(pairs ...string) string {
	var out []string
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, sText.Render(pairs[i])+" "+sDim.Render(pairs[i+1]))
	}
	return strings.Join(out, sFaint.Render("  ·  "))
}

type view int

const (
	viewList view = iota
	viewDetail
)

type logsMsg struct {
	id    string
	lines []model.LogLine
}

type flashMsg string

type tickMsg time.Time

type Model struct {
	client    *Client
	stream    chan streamMsg
	state     model.State
	connected bool
	err       string

	width, height int
	cursor        int
	view          view

	key       string
	repoIdx   int
	filter    int
	seen      int
	reviewIdx int
	tab       int
	logs      map[string][]model.LogLine
	vp        viewport.Model
	follow    bool

	providers *model.Providers
	confirm   *confirmState

	spin    spinner.Model
	flash   string
	flashAt time.Time
}

func New(client *Client) Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = sViolet
	return Model{
		client: client,
		stream: make(chan streamMsg, 1024),
		logs:   map[string][]model.LogLine{},
		spin:   sp,
		follow: true,
	}
}

func (m Model) Init() tea.Cmd {
	go m.client.Stream(m.stream)
	return tea.Batch(m.wait(), m.spin.Tick, tick(), m.loadProviders())
}

func (m Model) wait() tea.Cmd {
	return func() tea.Msg { return <-m.stream }
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp.Width = msg.Width
		m.vp.Height = max(1, msg.Height-m.detailChrome())
		m.refreshDetail()
		return m, nil

	case streamMsg:
		switch {
		case msg.err != nil:
			m.connected = false
			m.err = msg.err.Error()
		case msg.state != nil:
			m.connected = true
			m.err = ""
			m.state = *msg.state
			sortPRs(m.state.PRs)
			m.cursor = min(m.cursor, max(0, len(m.visible())-1))
			var seen tea.Cmd
			if m.view == viewDetail {
				if p, ok := m.selected(); !ok {
					m.view = viewList
				} else if p.Unread() {
					seen = m.markSeen(p)
				}
			}
			m.refreshDetail()
			return m, tea.Batch(m.wait(), seen)
		case msg.log != nil:
			id := msg.log.ReviewID
			if _, ok := m.logs[id]; ok {
				m.logs[id] = append(m.logs[id], *msg.log)
				if r := m.currentReview(); r != nil && r.ID == id {
					m.refreshDetail()
				}
			}
		}
		return m, m.wait()

	case logsMsg:
		live := m.logs[msg.id]
		m.logs[msg.id] = mergeLogs(msg.lines, live)
		m.refreshDetail()
		return m, nil

	case openConfirmMsg:
		return m.openConfirm(msg.pr)

	case planMsg:
		if m.confirm != nil && m.confirm.pr.Key() == msg.key {
			if msg.err != nil {
				m.confirm.err = msg.err.Error()
			} else {
				m.confirm.plan = &msg.plan
			}
		}
		return m, nil

	case providersMsg:
		m.providers = &msg.p
		if m.confirm != nil && !m.confirm.ready {
			m.confirm.init(msg.p)
		}
		return m, nil

	case flashMsg:
		m.flash = string(msg)
		m.flashAt = time.Now()
		return m, nil

	case tickMsg:
		if m.flash != "" && time.Since(m.flashAt) > 4*time.Second {
			m.flash = ""
		}
		return m, tick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if m.confirm != nil {
			return m.keyConfirm(msg)
		}
		if m.view == viewDetail {
			return m.keyDetail(msg)
		}
		return m.keyList(msg)

	case tea.MouseMsg:
		if m.confirm != nil {
			return m, nil
		}
		if m.view == viewDetail {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			m.follow = m.vp.AtBottom()
			return m, cmd
		}
		return m.mouseList(msg)
	}
	return m, nil
}

func (m Model) keyDetail(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	p, ok := m.selected()
	if !ok {
		m.view = viewList
		return m, nil
	}
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "h", "left", "backspace":
		m.view = viewList
		return m, nil
	case "tab":
		m.tab = 1 - m.tab
		m.follow = true
		m.refreshDetail()
		return m, nil
	case "[":
		if m.reviewIdx > 0 {
			m.reviewIdx--
			m.follow = true
			m.refreshDetail()
			return m, m.loadLog()
		}
		return m, nil
	case "]":
		if m.reviewIdx < len(p.Reviews)-1 {
			m.reviewIdx++
			m.follow = true
			m.refreshDetail()
			return m, m.loadLog()
		}
		return m, nil
	case "c":
		if r := m.currentReview(); r != nil && r.CommentURL != "" {
			return m, openURL(r.CommentURL)
		}
		return m, nil
	}
	if cmd := m.prAction(p, k.String()); cmd != nil {
		return m, cmd
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(k)
	m.follow = m.vp.AtBottom()
	return m, cmd
}

func (m Model) prAction(p model.PR, key string) tea.Cmd {
	act := func(action, done string) tea.Cmd {
		return func() tea.Msg {
			if err := m.client.Action(p.Repo, p.Number, action); err != nil {
				return flashMsg("error: " + err.Error())
			}
			return flashMsg(fmt.Sprintf("%s #%d", done, p.Number))
		}
	}
	switch key {
	case "r", " ":
		return func() tea.Msg { return openConfirmMsg{pr: p} }
	case "x":
		return act("cancel", "canceled")
	case "R":
		return act("refresh", "refreshing")
	case "o":
		return openURL(p.URL)
	}
	return nil
}

func (m Model) openDetail(p model.PR) (tea.Model, tea.Cmd) {
	m.view = viewDetail
	m.key = p.Key()
	m.reviewIdx = len(p.Reviews) - 1
	m.seen = len(p.Reviews)
	m.tab = 0
	if r := p.LastReview(); r != nil && r.Status == model.StatusReviewing {
		m.tab = 1
	}
	m.follow = true
	m.vp = viewport.New(m.width, max(1, m.height-m.detailChrome()))
	m.refreshDetail()
	if p.Unread() {
		return m, tea.Batch(m.loadLog(), m.markSeen(p))
	}
	return m, m.loadLog()
}

func (m Model) markSeen(p model.PR) tea.Cmd {
	return func() tea.Msg {
		m.client.Action(p.Repo, p.Number, "seen")
		return nil
	}
}

func (m Model) loadLog() tea.Cmd {
	r := m.currentReview()
	if r == nil {
		return nil
	}
	id := r.ID
	if _, ok := m.logs[id]; !ok {
		m.logs[id] = nil
	}
	return func() tea.Msg {
		lines, err := m.client.Log(id)
		if err != nil {
			return flashMsg("error: " + err.Error())
		}
		return logsMsg{id: id, lines: lines}
	}
}

func (m Model) selected() (model.PR, bool) {
	for _, p := range m.state.PRs {
		if p.Key() == m.key {
			return p, true
		}
	}
	return model.PR{}, false
}

func (m Model) currentReview() *model.Review {
	p, ok := m.selected()
	if !ok || len(p.Reviews) == 0 {
		return nil
	}
	i := m.reviewIdx
	if i < 0 || i >= len(p.Reviews) {
		i = len(p.Reviews) - 1
	}
	return &p.Reviews[i]
}

func (m *Model) refreshDetail() {
	if m.view != viewDetail {
		return
	}
	p, ok := m.selected()
	if !ok {
		return
	}
	if m.reviewIdx >= len(p.Reviews) || m.reviewIdx < 0 {
		m.reviewIdx = len(p.Reviews) - 1
	}
	if len(p.Reviews) > m.seen {
		if m.reviewIdx >= m.seen-1 {
			m.reviewIdx = len(p.Reviews) - 1
			m.tab = 1
			m.follow = true
			if id := p.Reviews[m.reviewIdx].ID; m.logs[id] == nil {
				m.logs[id] = []model.LogLine{}
			}
		}
		m.seen = len(p.Reviews)
	}
	w := max(20, m.width-2)
	var content string
	r := m.currentReview()
	switch {
	case r == nil:
		content = sDim.Render("No reviews yet.")
	case m.tab == 0:
		content = renderFindings(*r, w)
	default:
		content = renderLog(m.logs[r.ID], w)
	}
	m.vp.Height = max(1, m.height-m.detailChrome())
	m.vp.Width = m.width
	m.vp.SetContent(indent(content, " "))
	if m.follow && m.tab == 1 {
		m.vp.GotoBottom()
	}
}

func (m Model) detailChrome() int { return 9 }

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.confirm != nil {
		return m.viewConfirm()
	}
	if m.view == viewDetail {
		return m.viewDetail()
	}
	return m.viewList()
}

func (m Model) header() string {
	left := sBold.Render("prmax")
	right := ""
	n := 0
	for _, p := range m.state.PRs {
		if p.Status == model.StatusReviewing || p.Status == model.StatusQueued {
			n++
		}
	}
	if n > 0 {
		right = m.spin.View() + " " + sDim.Render(fmt.Sprintf("%d reviewing", n))
	}
	if !m.connected {
		right = sRed.Render("daemon offline")
	}
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-4)
	return "  " + left + strings.Repeat(" ", gap) + right + "  "
}

func (m Model) footer(k string) string {
	if m.flash != "" {
		return "  " + sTitle.Render(m.flash)
	}
	return "  " + k
}

func (m Model) statusBits(p model.PR) (string, string) {
	r := p.LastReview()
	switch p.Status {
	case model.StatusReviewing:
		label := "reviewing"
		if r != nil && r.Kind == model.KindIncremental {
			label = "re-reviewing"
		}
		return m.spin.View(), sViolet.Render(label)
	case model.StatusQueued:
		return sViolet.Render("◌"), sViolet.Render("queued")
	case model.StatusClean:
		return sGreen.Render("✓"), sGreen.Render("passed")
	case model.StatusFindings:
		n := 0
		if r := lastDone(p); r != nil {
			n = len(r.Findings)
		}
		return sOrange.Render("△"), sOrange.Render(issueCount(n))
	case model.StatusFailed:
		return sRed.Render("✕"), sRed.Render("failed")
	case model.StatusOutdated:
		label := "updated"
		if r := lastDone(p); r != nil && len(r.Findings) > 0 {
			label = "updated · " + issueCount(len(r.Findings))
		}
		return sTeal.Render("↻"), sTeal.Render(label)
	}
	if p.Draft {
		return sTitle.Render("○"), sDim.Render("draft")
	}
	return sTitle.Render("○"), sDim.Render("not reviewed")
}

func (m Model) viewDetail() string {
	p, _ := m.selected()
	var b strings.Builder
	b.WriteString(m.header() + "\n")
	b.WriteString(rule(m.width) + "\n")
	icon, info := m.statusBits(p)
	b.WriteString("  " + icon + "  " + sDim.Render(fmt.Sprintf("%s #%d", shortRepo(p.Repo), p.Number)) + "  " + sBold.Render(clip(p.Title, max(10, m.width-30))) + "\n")
	b.WriteString("     " + sDim.Render(fmt.Sprintf("%s → %s  ·  %s  ·  %s", p.Branch, p.BaseBranch, p.Author, short(p.HeadSHA))) + "  " + info + "\n")
	b.WriteString(m.roundsBar(p) + "\n")
	b.WriteString(m.vp.View() + "\n")
	b.WriteString(rule(m.width) + "\n")
	b.WriteString(m.footer(keys("tab", "findings/log", "[ ]", "round", "c", "comment", "o", "browser", "r", "review", "x", "cancel", "esc", "back")))
	return b.String()
}

func (m Model) roundsBar(p model.PR) string {
	var t []string
	for i, name := range []string{"Findings", "Log"} {
		if i == m.tab {
			t = append(t, sTabOn.Render(name))
		} else {
			t = append(t, sTab.Render(name))
		}
	}
	line := "\n  " + strings.Join(t, "   ")
	if r := m.currentReview(); r != nil {
		parts := []string{fmt.Sprintf("%d/%d", m.reviewIdx+1, len(p.Reviews))}
		if r.Round > 0 {
			parts = append(parts, fmt.Sprintf("round %d", r.Round))
		}
		if r.SinceSHA != "" {
			parts = append(parts, "since "+short(r.SinceSHA))
		}
		if r.ModelLabel != "" {
			parts = append(parts, r.ModelLabel)
		}
		parts = append(parts, r.StartedAt.Local().Format("Jan 2 15:04"))
		if !r.FinishedAt.IsZero() {
			parts = append(parts, r.FinishedAt.Sub(r.StartedAt).Round(time.Second).String())
		}
		if r.CostUSD > 0 {
			parts = append(parts, fmt.Sprintf("$%.2f", r.CostUSD))
		}
		line += "      " + sDim.Render(strings.Join(parts, "  ·  "))
	}
	return line + "\n"
}

func renderFindings(r model.Review, w int) string {
	var b strings.Builder
	if r.Status == model.StatusReviewing {
		b.WriteString(" " + sTitle.Render("Review in progress. Press tab for the live log.") + "\n")
		return b.String()
	}
	if r.Error != "" {
		b.WriteString(" " + sRed.Render(lipgloss.NewStyle().Width(w-4).Render(r.Error)) + "\n\n")
	}
	if r.Status != model.StatusClean && r.Status != model.StatusFindings {
		return b.String()
	}
	if len(r.Findings) == 0 {
		b.WriteString(" " + sGreen.Render("No findings.") + "\n\n")
	}
	text := lipgloss.NewStyle().Width(w - 6)
	for _, c := range model.Categories {
		fs := r.ByCategory(c.ID)
		if len(fs) == 0 {
			continue
		}
		b.WriteString(" " + sBold.Render(c.Label) + "\n")
		for _, f := range fs {
			loc := f.File
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			head := "   " + sBold.Render(strings.TrimSpace(f.Title)) + "  " + sTitle.Render(loc)
			if f.StillOpen {
				head += "  " + sOrange.Render("still open")
			}
			b.WriteString(head + "\n")
			b.WriteString(indent(sText.Render(text.Render(strings.TrimSpace(f.Text))), "   ") + "\n")
		}
		b.WriteString("\n")
	}
	if r.CommentURL != "" {
		b.WriteString(" " + sDim.Render("posted: "+r.CommentURL) + "\n")
	}
	return b.String()
}

func renderLog(lines []model.LogLine, w int) string {
	if len(lines) == 0 {
		return sDim.Render(" Waiting for output…")
	}
	var b strings.Builder
	textW := max(10, w-12)
	for _, l := range lines {
		ts := sDim.Render(l.Time.Local().Format("15:04:05"))
		var body string
		switch l.Kind {
		case "tool":
			body = sTitle.Render("› ") + sDim.Render(clip(l.Text, textW))
		case "text":
			body = indent(sText.Render(lipgloss.NewStyle().Width(textW).Render(l.Text)), "")
			body = strings.ReplaceAll(body, "\n", "\n           ")
		case "error":
			body = sRed.Render(clip(l.Text, textW*3))
		case "done":
			body = sGreen.Render(l.Text)
		default:
			body = sDim.Render(l.Text)
		}
		b.WriteString(" " + ts + "  " + body + "\n")
	}
	return b.String()
}

func mergeLogs(history, live []model.LogLine) []model.LogLine {
	if len(history) == 0 {
		return live
	}
	last := history[len(history)-1].Time
	out := history
	for _, l := range live {
		if l.Time.After(last) {
			out = append(out, l)
		}
	}
	return out
}

func sortPRs(prs []model.PR) {
	sort.SliceStable(prs, func(i, j int) bool {
		if !prs[i].UpdatedAt.Equal(prs[j].UpdatedAt) {
			return prs[i].UpdatedAt.After(prs[j].UpdatedAt)
		}
		return prs[i].Key() < prs[j].Key()
	})
}

func lastDone(p model.PR) *model.Review {
	for i := len(p.Reviews) - 1; i >= 0; i-- {
		if s := p.Reviews[i].Status; s == model.StatusClean || s == model.StatusFindings {
			return &p.Reviews[i]
		}
	}
	return nil
}

func statusWord(r model.Review) string {
	switch r.Status {
	case model.StatusClean:
		return sGreen.Render("clean")
	case model.StatusFindings:
		return sOrange.Render(fmt.Sprintf("%d findings", len(r.Findings)))
	case model.StatusFailed:
		return sRed.Render("failed")
	case model.StatusReviewing:
		return sTitle.Render("running")
	}
	return r.Status
}

func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return flashMsg("no url")
		}
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		return flashMsg("opened in browser")
	}
}

func shortRepo(r string) string { return r[strings.LastIndex(r, "/")+1:] }

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func indent(s, pre string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = pre + lines[i]
	}
	return strings.Join(lines, "\n")
}

func ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func issueCount(n int) string {
	if n == 1 {
		return "1 issue"
	}
	return fmt.Sprintf("%d issues", n)
}
