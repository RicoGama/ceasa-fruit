package mail

import (
	"bytes"
	"embed"
	htmltemplate "html/template"
	texttemplate "text/template"
)

// Os modelos ficam dentro do binário (como as migrations).
//
// Cada e-mail tem dois arquivos:
//   - <nome>.html.tmpl: versão HTML, que usa o layout.html.tmpl
//   - <nome>.txt.tmpl:  versão em texto puro
//
// Cada um define o bloco "subject" (assunto) e o bloco "body".
//
//go:embed templates/*.tmpl
var templatesFS embed.FS

// html/template escapa os dados para HTML (proteção contra injeção);
// text/template não escapa nada, o certo para texto puro e assunto.
var (
	htmlTemplates = htmltemplate.Must(htmltemplate.New("").Funcs(htmltemplate.FuncMap{"dict": dict}).
			ParseFS(templatesFS, "templates/*.html.tmpl"))
	textTemplates = texttemplate.Must(texttemplate.ParseFS(templatesFS, "templates/*.txt.tmpl"))
)

// Render monta a mensagem a partir do modelo `name` (ex.: "welcome").
func Render(name, to, toName string, data any) (Message, error) {
	msg := Message{To: to, ToName: toName}

	var subject, text, html bytes.Buffer
	// Cada arquivo de texto define "<nome>.subject" e "<nome>.text".
	if err := textTemplates.ExecuteTemplate(&subject, name+".subject", data); err != nil {
		return msg, err
	}
	if err := textTemplates.ExecuteTemplate(&text, name+".text", data); err != nil {
		return msg, err
	}
	// O HTML usa o layout, que chama o bloco "<nome>.html" para o miolo.
	if err := htmlTemplates.ExecuteTemplate(&html, name+".html", map[string]any{
		"Subject": subject.String(),
		"Data":    data,
	}); err != nil {
		return msg, err
	}

	msg.Subject = subject.String()
	msg.Text = text.String()
	msg.HTML = html.String()
	return msg, nil
}

// dict monta um mapa a partir de pares chave/valor, para passar mais de um
// valor a um bloco: {{template "layout.button" (dict "URL" x "Label" "y")}}.
func dict(pairs ...any) map[string]any {
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1]
	}
	return m
}
