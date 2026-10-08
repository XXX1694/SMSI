package mail

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"

	"github.com/socialos/backend/internal/application/port"
)

// ProductName is shown in every message.
const ProductName = "SocialOS"

// Template names.
const (
	VerifyEmail     = "verify_email"
	ResetPassword   = "reset_password"
	PasswordChanged = "password_changed"
	AccountDeleted  = "account_deleted"
	ExportReady     = "export_ready"
)

//go:embed templates/*.tmpl
var files embed.FS

// Data is the input of every template. Link and ExpiresIn are unused by
// templates that do not need them.
type Data struct {
	Product   string
	Link      string
	ExpiresIn string
}

var subjects = map[string]string{
	VerifyEmail:     "Verify your email for " + ProductName,
	ResetPassword:   "Reset your " + ProductName + " password",
	PasswordChanged: "Your " + ProductName + " password was changed",
	AccountDeleted:  "Your " + ProductName + " account was deleted",
	ExportReady:     "Your " + ProductName + " data export is ready",
}

type pair struct {
	text *texttemplate.Template
	html *htmltemplate.Template
}

var parsed = func() map[string]pair {
	out := map[string]pair{}
	for name := range subjects {
		out[name] = pair{
			text: texttemplate.Must(texttemplate.ParseFS(files, "templates/"+name+".txt.tmpl")),
			html: htmltemplate.Must(htmltemplate.ParseFS(files, "templates/"+name+".html.tmpl")),
		}
	}
	return out
}()

// Render builds the message for a template. The HTML part is escaped by
// html/template; the text part is not HTML and is rendered verbatim.
func Render(template, to string, d Data) (port.Message, error) {
	p, ok := parsed[template]
	if !ok {
		return port.Message{}, fmt.Errorf("mail: unknown template %q", template)
	}
	d.Product = ProductName
	var text, html bytes.Buffer
	if err := p.text.Execute(&text, d); err != nil {
		return port.Message{}, fmt.Errorf("mail: render %s text: %w", template, err)
	}
	if err := p.html.Execute(&html, d); err != nil {
		return port.Message{}, fmt.Errorf("mail: render %s html: %w", template, err)
	}
	return port.Message{To: to, Subject: subjects[template], Text: text.String(), HTML: html.String(), Template: template}, nil
}
