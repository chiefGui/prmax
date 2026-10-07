package daemon

import (
	"context"
	"fmt"
	"html"
	"log"
	"os/exec"
	"strings"
	"time"

	"prmax/internal/model"
	"prmax/internal/proc"
)

const toastApp = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

func notify(p model.PR, r model.Review) {
	var outcome string
	switch r.Status {
	case model.StatusClean:
		outcome = "passed"
	case model.StatusFindings:
		outcome = fmt.Sprintf("%d issues", len(r.Findings))
		if len(r.Findings) == 1 {
			outcome = "1 issue"
		}
	case model.StatusFailed:
		outcome = "failed"
	default:
		return
	}
	link := r.CommentURL
	if link == "" {
		link = p.URL
	}
	title := fmt.Sprintf("%s #%d · Round %d %s", p.Repo[strings.LastIndex(p.Repo, "/")+1:], p.Number, r.Round, outcome)
	xml := fmt.Sprintf(`<toast activationType="protocol" launch="%s"><visual><binding template="ToastGeneric"><text>%s</text><text>%s</text></binding></visual></toast>`,
		html.EscapeString(link), html.EscapeString(title), html.EscapeString(p.Title))
	script := `[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$x = New-Object Windows.Data.Xml.Dom.XmlDocument
$x.LoadXml('` + strings.ReplaceAll(xml, "'", "''") + `')
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('` + toastApp + `').Show([Windows.UI.Notifications.ToastNotification]::new($x))`
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", "-")
		cmd.Stdin = strings.NewReader(script)
		proc.Hide(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("notify: %v %s", err, strings.TrimSpace(string(out)))
		}
	}()
}
